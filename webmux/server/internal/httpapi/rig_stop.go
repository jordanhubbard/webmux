package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
)

type rigStopExpected struct {
	NodeID    string `json:"nodeId"`
	SessionID string `json:"sessionId"`
}
type rigStopReview struct {
	Contract        string          `json:"contract"`
	RigID           string          `json:"rigId"`
	LogicalID       string          `json:"logicalId"`
	SessionName     string          `json:"sessionName"`
	TmuxServerFlag  string          `json:"tmuxServerFlag"`
	TmuxServerValue string          `json:"tmuxServerValue"`
	Port            int             `json:"port"`
	Runtime         string          `json:"runtime"`
	ManagerPID      int             `json:"managerPid"`
	ManagerIdentity string          `json:"managerIdentity"`
	Expected        rigStopExpected `json:"expected"`
}
type rigStopPreview struct {
	ID        string        `json:"id"`
	Owner     string        `json:"owner"`
	Target    string        `json:"target"`
	Reason    string        `json:"reason"`
	ExpiresAt time.Time     `json:"expiresAt"`
	Review    rigStopReview `json:"review"`
}
type rigStopReceipt struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Code      string `json:"code"`
	Posted    *bool  `json:"posted,omitempty"`
	StopPhase string `json:"stopPhase"`
}

var rigStopRecords sync.Mutex
var rigStopID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var rigStopULID = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

func validRigStopReview(value rigStopReview) bool {
	_, err := rigTmuxServerArgs(value.TmuxServerFlag, value.TmuxServerValue)
	return err == nil && value.TmuxServerFlag == "-S" && value.Contract == "webmux-rig-stop/v1" &&
		value.ManagerPID > 0 && rigStopID.MatchString(value.ManagerIdentity) &&
		value.Port >= 1024 && value.Port <= 65535 && rigStopULID.MatchString(value.RigID) &&
		rigStopULID.MatchString(value.Expected.NodeID) && rigStopULID.MatchString(value.Expected.SessionID) &&
		localRigSeatID(value.RigID, value.LogicalID) != "" && rigTmuxTarget.MatchString(value.SessionName)
}

// Rig Stop is a local, native manager action. Observed remote seats and other
// integrations retain their execution host's lifecycle controls.
func (s *Server) rigStopAccess(w http.ResponseWriter, owner string) bool {
	if !s.openRigAccess(w, owner) {
		return false
	}
	if runtime.GOOS == "windows" || s.coordinationProvider(owner, "rig") == nil {
		writeError(w, 409, "Checked local Rig Stop is unavailable. Use Rig on the execution host; retained viewers and logs are preserved.")
		return false
	}
	return true
}

func decodeRigStopInput(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, 400, "Invalid checked Stop request.")
		return false
	}
	return true
}

func rigStopModule(ctx context.Context, home, owner, action string, input any, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	raw, err := openrigcoord.Run(ctx, home, owner, action, string(data))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, output)
}

func (s *Server) previewRigStop(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.rigStopAccess(w, owner) {
		return
	}
	var input struct {
		Provider string `json:"provider"`
		Target   string `json:"target"`
		Reason   string `json:"reason"`
	}
	if !decodeRigStopInput(w, r, &input) {
		return
	}
	if input.Provider != "rig" || !strings.HasPrefix(input.Target, localRigPrefix) || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 1024 || strings.ContainsFunc(input.Reason, unicode.IsControl) {
		writeError(w, 400, "Select a local canonical Rig seat and give a reason for stopping it. Remote provider Stop is unavailable here.")
		return
	}
	provider := s.coordinationProvider(owner, "rig").(*rigProvider)
	readOnly := *provider
	readOnly.factsOnly = true
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	raw, err := readOnly.rawStatus(ctx)
	if err != nil {
		writeError(w, 409, "Rig could not verify the selected seat. Nothing was stopped.")
		return
	}
	seat, err := resolveLocalRigSeat(raw, input.Target)
	if err != nil || !seat.Openable {
		writeError(w, 409, "The selected canonical Rig seat is unavailable. Nothing was stopped.")
		return
	}
	var review rigStopReview
	selection := map[string]any{"rigId": seat.RigID, "logicalId": seat.LogicalID, "sessionName": seat.CanonicalSessionName, "tmuxServerFlag": seat.TmuxServerFlag, "tmuxServerValue": seat.TmuxServerValue}
	if err := rigStopModule(ctx, s.store.Home, owner, "stop-preview", selection, &review); err != nil || !validRigStopReview(review) {
		message := "Rig could not approve a checked local seat Stop. Adopted seats, unsupported manager versions and unverified destinations cannot be stopped here. Nothing was stopped."
		if err != nil {
			switch strings.TrimSpace(err.Error()) {
			case "claimed_session":
				message = "This seat was adopted by Rig. Rig refuses to stop it; use its original lifecycle owner. Nothing was stopped."
			case "unsupported_version", "unsupported_manager_build":
				message = "Checked local Stop requires OpenRig 0.7.18 (3bf87c62) for both CLI and daemon. Nothing was stopped."
			case "local_manager_unverified":
				message = "Cannot verify Rig as a local daemon owned by this WebMux service account. Local listener and process evidence (lsof and ps) is required. Nothing was stopped."
			case "selection_changed":
				message = "The Rig seat changed while it was being reviewed. Review the current seat again. Nothing was stopped."
			}
		}
		writeError(w, 409, message)
		return
	}
	preview, err := newRigStopPreview(s.store.Home, owner, input.Target, strings.TrimSpace(input.Reason), review)
	if err != nil {
		writeError(w, 500, "Cannot retain the Stop review. Nothing was stopped.")
		return
	}
	// Owner and loopback port are private ledger metadata, not browser inputs.
	writeJSON(w, 200, map[string]any{"id": preview.ID, "target": preview.Target, "reason": preview.Reason, "expiresAt": preview.ExpiresAt,
		"project": review.RigID, "seat": review.LogicalID, "sessionName": review.SessionName, "runtime": review.Runtime,
		"tmuxServer": review.TmuxServerValue, "nodeId": review.Expected.NodeID, "sessionId": review.Expected.SessionID,
		"effect": "Stop this Rig seat's local tmux session on the WebMux host and disconnect all its viewers. Jobs outside that session, including remote providers, may continue. No remote provider stop is performed."})
}

func (s *Server) executeRigStop(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.rigStopAccess(w, owner) {
		return
	}
	var input struct {
		ID      string `json:"id"`
		Confirm bool   `json:"confirm"`
	}
	if !decodeRigStopInput(w, r, &input) {
		return
	}
	if !input.Confirm || !rigStopID.MatchString(input.ID) {
		writeError(w, 400, "Confirm the retained local Rig Stop review before proceeding.")
		return
	}
	result, err := consumeRigStop(r.Context(), s.store.Home, owner, input.ID, func(ctx context.Context, preview rigStopPreview) (rigStopReceipt, error) {
		if s.openRigPermission(owner) != nil || s.coordinationProvider(owner, "rig") == nil {
			posted := false
			return rigStopReceipt{State: "refused", Code: "access_changed", Posted: &posted, StopPhase: "none"}, nil
		}
		// The durable claim exists before even this helper's read-only checks.
		// Losing the controller/helper/response therefore cannot trigger retry.
		var result rigStopReceipt
		err := rigStopModule(ctx, s.store.Home, owner, "stop-execute", map[string]any{"review": preview.Review, "reason": preview.Reason, "operator": owner}, &result)
		return result, err
	})
	if err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 200, result)
}

func rigStopRoot(home, owner string) (string, string) {
	key := sha256.Sum256([]byte(owner))
	root := filepath.Join(home, "data", "rig-stops")
	return root, filepath.Join(root, "owners", hex.EncodeToString(key[:]))
}

// Exclusive, immutable records plus file/directory sync precede lifecycle
// dispatch. A missing or partial completion always leaves the claim consumed.
func writeRigStopRecord(home, name string, value any) error {
	boundary, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	name, err = filepath.Abs(name)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(boundary, name)
	if err != nil || !filepath.IsLocal(relative) {
		return errors.New("Stop record escaped private storage")
	}
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(value)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	// Sync every directory link through the storage root's parent. Another
	// process may have just created an ancestor without syncing it yet.
	for current := filepath.Dir(name); ; current = filepath.Dir(current) {
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
			return nil
		}
	}
}

func readRigStopRecord(name string, value any) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return errors.New("invalid Stop record")
	}
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid Stop record")
	}
	return nil
}

func newRigStopPreview(home, owner, target, reason string, review rigStopReview) (rigStopPreview, error) {
	if !validRigStopReview(review) || target != localRigSeatID(review.RigID, review.LogicalID) || owner == "" || reason == "" {
		return rigStopPreview{}, errors.New("invalid Stop review")
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return rigStopPreview{}, err
	}
	preview := rigStopPreview{ID: hex.EncodeToString(nonce[:]), Owner: owner, Target: target, Reason: reason, ExpiresAt: time.Now().UTC().Add(3 * time.Minute), Review: review}
	_, directory := rigStopRoot(home, owner)
	err := writeRigStopRecord(home, filepath.Join(directory, preview.ID+".review.json"), preview)
	return preview, err
}

func uncertainRigStop(id string) rigStopReceipt {
	return rigStopReceipt{ID: id, State: "uncertain", Code: "in_flight_or_unknown", StopPhase: "unknown"}
}

func consumeRigStop(ctx context.Context, home, owner, id string, dispatch func(context.Context, rigStopPreview) (rigStopReceipt, error)) (rigStopReceipt, error) {
	if !rigStopID.MatchString(id) || dispatch == nil {
		return rigStopReceipt{}, errors.New("Invalid retained Stop review.")
	}
	root, directory := rigStopRoot(home, owner)
	var preview rigStopPreview
	if readRigStopRecord(filepath.Join(directory, id+".review.json"), &preview) != nil || preview.Owner != owner || preview.ID != id || !validRigStopReview(preview.Review) || preview.Target != localRigSeatID(preview.Review.RigID, preview.Review.LogicalID) {
		return rigStopReceipt{}, errors.New("The Stop review is unavailable for this owner. Nothing was dispatched.")
	}
	resultPath := filepath.Join(directory, id+".result.json")
	readResult := func() (rigStopReceipt, bool) {
		var result rigStopReceipt
		err := readRigStopRecord(resultPath, &result)
		if errors.Is(err, os.ErrNotExist) {
			return result, false
		}
		if err != nil || result.ID != id {
			return uncertainRigStop(id), true
		}
		return result, true
	}
	rigStopRecords.Lock()
	if result, exists := readResult(); exists {
		rigStopRecords.Unlock()
		return result, nil
	}
	// Exclude mutable endpoint, socket, labels and owner: uncertainty must not
	// be bypassed by another admin or a daemon port change for this enrollment.
	key := sha256.Sum256([]byte(preview.Review.RigID + "\x00" + preview.Review.Expected.NodeID + "\x00" + preview.Review.Expected.SessionID))
	claim := filepath.Join(root, "claims", hex.EncodeToString(key[:])+".json")
	if _, err := os.Lstat(claim); err == nil {
		rigStopRecords.Unlock()
		return uncertainRigStop(id), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		rigStopRecords.Unlock()
		return rigStopReceipt{}, errors.New("Cannot inspect earlier Stop receipts. No new lifecycle request was dispatched; inspect Rig.")
	}
	if time.Now().After(preview.ExpiresAt) || preview.ExpiresAt.After(time.Now().Add(3*time.Minute+time.Second)) || ctx.Err() != nil {
		rigStopRecords.Unlock()
		return rigStopReceipt{}, errors.New("The Stop review expired or was cancelled. Review the current seat again; no new request was dispatched.")
	}
	if err := writeRigStopRecord(home, claim, map[string]string{"id": id}); err != nil {
		rigStopRecords.Unlock()
		if errors.Is(err, os.ErrExist) {
			return uncertainRigStop(id), nil
		}
		return rigStopReceipt{}, errors.New("Cannot durably claim this Stop. No lifecycle request was dispatched.")
	}
	// Close the cross-process race where a prior confirmed no-effect attempt
	// completed and released its claim between the first read and this claim.
	if result, exists := readResult(); exists {
		_ = os.Remove(claim)
		rigStopRecords.Unlock()
		return result, nil
	}
	rigStopRecords.Unlock()
	result, err := dispatch(ctx, preview)
	if err != nil || result.Posted == nil || (result.State != "stopped" && result.State != "refused" && result.State != "uncertain") || result.State == "refused" && result.StopPhase != "none" || result.State == "stopped" && (!*result.Posted || result.StopPhase != "kill_attempted") {
		result = uncertainRigStop(id)
	}
	result.ID = id
	rigStopRecords.Lock()
	defer rigStopRecords.Unlock()
	if err := writeRigStopRecord(home, resultPath, result); err != nil {
		return uncertainRigStop(id), nil
	}
	if result.State == "refused" && result.StopPhase == "none" {
		// A new explicit review may retry a positively confirmed no-effect
		// refusal. This request ID itself always returns its immutable result.
		if err := os.Remove(claim); err != nil {
			return uncertainRigStop(id), nil
		}
	}
	return result, nil
}
