package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// The UUID is allocated by the browser before any network request. The intent,
// dispatch/fence claim and outcome are immutable: response loss never grants a
// second right to invoke the owner. Within this data home claims bind the UUID, not an owner
// namespace which another administrator could use to redispatch the same ID.
type coordinationSendInput struct {
	RequestID string `json:"requestId"`
	Scope     string `json:"scope"`
	Provider  string `json:"provider"`
	coordinationSendRequest
}

type coordinationSendOutcome struct {
	coordinationSendResult
	RequestID    string                 `json:"requestId"`
	RequestState string                 `json:"requestState"`
	RequestCode  string                 `json:"requestCode"`
	Request      *coordinationSendInput `json:"request,omitempty"`
}

type coordinationSendIntent struct {
	Owner string                `json:"owner"`
	Hash  string                `json:"hash"`
	Input coordinationSendInput `json:"input"`
}

type coordinationSendClaim struct {
	Owner string `json:"owner"`
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Hash  string `json:"hash"`
}

type coordinationSendRecord struct {
	Owner   string                  `json:"owner"`
	Outcome coordinationSendOutcome `json:"outcome"`
}

type coordinationSendRefusal string

func (e coordinationSendRefusal) Error() string { return string(e) }

func coordinationSendScope(home, owner string) string {
	absolute, err := filepath.Abs(home)
	if err == nil {
		home = absolute
	}
	sum := sha256.Sum256([]byte(home + "\x00" + owner))
	return hex.EncodeToString(sum[:])
}

func coordinationSendPath(home, id, kind string) string {
	return filepath.Join(home, "data", "coordination-sends", id+"."+kind+".json")
}

func coordinationSendHash(input coordinationSendInput) string {
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validCoordinationSendInput(input coordinationSendInput) bool {
	return rigStopID.MatchString(input.Scope) && creationOperationID.MatchString(input.RequestID) && input.Provider == "agent-deck" && input.Target != "" && len(input.Target) <= 1024 &&
		!strings.ContainsAny(input.Target, "\x00\r\n") && strings.TrimSpace(input.Message) != "" && len(input.Message) <= 256*1024
}

func readCoordinationSendRecord(home, path string, value any) error {
	boundary, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(boundary, path)
	if err != nil || !filepath.IsLocal(relative) {
		return errors.New("instruction record escaped private storage")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	// JSON escaping can expand a 256 KiB instruction. Never follow a record link.
	if !info.Mode().IsRegular() || info.Size() > 2*1024*1024 {
		return errors.New("invalid saved instruction record")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 2*1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid saved instruction record")
	}
	// A concurrent writer can have made a complete record visible before its
	// final Sync. Seal valid immutable bytes before trusting a fence or outcome;
	// in particular a cancelled UUID must survive a subsequent host crash.
	if err := f.Sync(); err != nil {
		return err
	}
	for current := filepath.Dir(path); ; current = filepath.Dir(current) {
		directory, err := os.Open(current)
		if err != nil {
			return err
		}
		err = directory.Sync()
		closeErr := directory.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if current == filepath.Dir(boundary) {
			break
		}
	}
	return nil
}

func pendingCoordinationSend(id, state, code string) coordinationSendOutcome {
	return coordinationSendOutcome{RequestID: id, RequestState: state, RequestCode: code}
}

// inspect never queries the owner or starts/restarts a delivery worker.
func inspectCoordinationSend(home, owner, id string) (coordinationSendOutcome, error) {
	if owner == "" || !creationOperationID.MatchString(id) {
		return coordinationSendOutcome{}, errors.New("Invalid saved instruction identity.")
	}
	out := pendingCoordinationSend(id, "unobserved", "not_observed")
	var intent coordinationSendIntent
	err := readCoordinationSendRecord(home, coordinationSendPath(home, id, "intent"), &intent)
	if err == nil {
		if intent.Owner != owner || intent.Input.Scope != coordinationSendScope(home, owner) || intent.Input.RequestID != id || !validCoordinationSendInput(intent.Input) || intent.Hash != coordinationSendHash(intent.Input) {
			return out, errors.New("Saved instruction unavailable for this owner.")
		}
		out.Request = &intent.Input
		out.Target = intent.Input.Target
		out.RequestState, out.RequestCode = "pending", "not_dispatched"
	} else if !errors.Is(err, os.ErrNotExist) {
		return out, errors.New("Cannot verify the saved instruction. Inspect its owner before further action.")
	}
	var claim coordinationSendClaim
	err = readCoordinationSendRecord(home, coordinationSendPath(home, id, "claim"), &claim)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil || claim.Owner != owner || claim.ID != id || (claim.Kind != "dispatch" && claim.Kind != "fence") || (claim.Kind == "dispatch" && (out.Request == nil || claim.Hash != intent.Hash)) {
		return out, errors.New("Cannot verify the saved instruction claim. No retry is authorized.")
	}
	out.RequestState, out.RequestCode = "uncertain", "in_flight_or_unknown"
	if claim.Kind == "fence" {
		out.RequestState, out.RequestCode = "cancelled", "fenced_before_dispatch"
	}
	var record coordinationSendRecord
	err = readCoordinationSendRecord(home, coordinationSendPath(home, id, "result"), &record)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil || record.Owner != owner || record.Outcome.RequestID != id || record.Outcome.Request != nil {
		// A partial outcome never becomes authority to invoke the owner again.
		out.RequestState, out.RequestCode = "uncertain", "outcome_unreadable"
		return out, nil
	}
	result := record.Outcome
	if result.RequestState != "completed" && result.RequestState != "refused" && result.RequestState != "uncertain" {
		return out, nil
	}
	if claim.Kind != "dispatch" || result.Target != out.Target {
		return out, nil
	}
	result.Request = out.Request
	return result, nil
}

func consumeCoordinationSend(ctx context.Context, home, owner string, input coordinationSendInput, dispatch func(context.Context, coordinationSendRequest) (coordinationSendResult, error)) (coordinationSendOutcome, error) {
	if owner == "" || !validCoordinationSendInput(input) || dispatch == nil {
		return coordinationSendOutcome{}, errors.New("Invalid coordination instruction.")
	}
	if input.Scope != coordinationSendScope(home, owner) {
		return coordinationSendOutcome{}, errors.New("The instruction owner changed. Reopen Coordination before sending.")
	}
	if runtime.GOOS == "windows" {
		return coordinationSendOutcome{}, errors.New("Durable Agent Deck instructions require a Linux or macOS WebMux host.")
	}
	current, err := inspectCoordinationSend(home, owner, input.RequestID)
	if err != nil {
		return current, err
	}
	if current.Request != nil && coordinationSendHash(*current.Request) != coordinationSendHash(input) {
		return current, errors.New("This request ID belongs to a different saved instruction. Inspect or dismiss the original.")
	}
	if current.RequestState != "pending" && current.RequestState != "unobserved" {
		current.Request = nil
		return current, nil
	}
	if err := ctx.Err(); err != nil {
		return current, err
	}
	intent := coordinationSendIntent{Owner: owner, Hash: coordinationSendHash(input), Input: input}
	// This existing primitive uses O_EXCL, a private file, file Sync and every
	// ancestor directory Sync. Its Stop-specific reader is intentionally not used
	// here: instruction records have a larger, separately bounded schema.
	if err := writeRigStopRecord(home, coordinationSendPath(home, input.RequestID, "intent"), intent); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return current, errors.New("Cannot retain the instruction. No provider call was made.")
		}
		var saved coordinationSendIntent
		if readCoordinationSendRecord(home, coordinationSendPath(home, input.RequestID, "intent"), &saved) != nil || saved.Owner != owner || saved.Hash != intent.Hash || coordinationSendHash(saved.Input) != intent.Hash {
			return current, errors.New("This request ID is unavailable or belongs to a different saved instruction.")
		}
	}
	claim := coordinationSendClaim{Owner: owner, ID: input.RequestID, Kind: "dispatch", Hash: intent.Hash}
	if err := writeRigStopRecord(home, coordinationSendPath(home, input.RequestID, "claim"), claim); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return current, errors.New("Cannot durably consume this instruction. No provider call was made.")
		}
		result, err := inspectCoordinationSend(home, owner, input.RequestID)
		result.Request = nil
		return result, err
	}
	result := pendingCoordinationSend(input.RequestID, "uncertain", "response_unavailable")
	result.Target = input.Target
	if ctx.Err() != nil {
		result.RequestState, result.RequestCode = "refused", "cancelled_before_dispatch"
	} else {
		native, err := dispatch(ctx, input.coordinationSendRequest)
		var refusal coordinationSendRefusal
		if errors.As(err, &refusal) {
			result.RequestState, result.RequestCode = "refused", string(refusal)
		} else if err == nil && native.Target == input.Target {
			result.coordinationSendResult = native
			result.RequestState, result.RequestCode = "completed", "owner_returned"
		}
	}
	if err := writeRigStopRecord(home, coordinationSendPath(home, input.RequestID, "result"), coordinationSendRecord{Owner: owner, Outcome: result}); err != nil {
		result = pendingCoordinationSend(input.RequestID, "uncertain", "outcome_not_retained")
		result.Target = input.Target
	}
	return result, nil
}

// A dismissed, previously unobserved UUID must be fenced, rather than forgotten:
// its original HTTP request could still be delayed in transit. The same O_EXCL
// claim decides between cancellation and dispatch; no read-then-delete race.
func dismissCoordinationSend(home, owner, id string) (coordinationSendOutcome, error) {
	if runtime.GOOS == "windows" {
		return coordinationSendOutcome{}, errors.New("Durable Agent Deck instructions require a Linux or macOS WebMux host.")
	}
	current, err := inspectCoordinationSend(home, owner, id)
	if err != nil {
		return current, err
	}
	if current.RequestState != "unobserved" && current.RequestState != "pending" {
		current.Request = nil
		return current, nil
	}
	claim := coordinationSendClaim{Owner: owner, ID: id, Kind: "fence"}
	if err := writeRigStopRecord(home, coordinationSendPath(home, id, "claim"), claim); err != nil && !errors.Is(err, os.ErrExist) {
		return current, errors.New("Cannot fence the original instruction. Keep its saved identity.")
	}
	result, err := inspectCoordinationSend(home, owner, id)
	result.Request = nil
	return result, err
}

func (s *Server) coordinationSendScope(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"scope": coordinationSendScope(s.store.Home, owner)})
}
func (s *Server) inspectCoordinationSend(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	if r.Header.Get("X-WebMux-Send-Scope") != coordinationSendScope(s.store.Home, owner) {
		writeError(w, 409, "The instruction owner changed. Reopen Coordination before proceeding.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	result, err := inspectCoordinationSend(s.store.Home, owner, r.PathValue("requestId"))
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) dismissCoordinationSend(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	if r.Header.Get("X-WebMux-Send-Scope") != coordinationSendScope(s.store.Home, owner) {
		writeError(w, 409, "The instruction owner changed. Reopen Coordination before proceeding.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	result, err := dismissCoordinationSend(s.store.Home, owner, r.PathValue("requestId"))
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
