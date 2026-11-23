package recovery

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"runtime"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
)

const maxRPCBytes = 256 << 10
const recoveryWorkerTimeout = 25 * time.Second

// RPCRequest travels through a local pipe or authenticated SSH stdin. Keys must
// never appear in a URL, command-line argument, terminal input or transcript.
type RPCRequest struct {
	CoordinationAccess    *coordinationclient.Access `json:"coordination_access,omitempty"`
	CoordinationChannel   string                     `json:"coordination_channel,omitempty"`
	CoordinationDirectory string                     `json:"coordination_directory,omitempty"`
	BrowserControl        bool                       `json:"browser_control,omitempty"`
	BrowserAck            string                     `json:"browser_ack,omitempty"`
	Protocol              int                        `json:"protocol"`
	SessionID             string                     `json:"session_id"`
	Key                   string                     `json:"key"`
	Operation             string                     `json:"operation"`
	Recording             *bool                      `json:"recording,omitempty"`
	Action                *RecoveryAction            `json:"action,omitempty"`
	Seed                  *Activity                  `json:"seed,omitempty"`
}

type RecoveryAction struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
	Revision uint64 `json:"revision"`
	Kind     string `json:"kind"` // native, replay, or shell
	Confirm  bool   `json:"confirm"`
	Context  string `json:"context,omitempty"`
}

type RPCResponse struct {
	RestartSelection      string                `json:"restart_selection,omitempty"`
	CoordinationDirectory string                `json:"coordination_directory,omitempty"`
	CoordinationPlatform  string                `json:"coordination_platform,omitempty"`
	CoordinationSocket    string                `json:"coordination_socket,omitempty"`
	CoordinationProject   *coordination.Binding `json:"coordination_project,omitempty"`
	BrowserRequest        *BrowserRequest       `json:"browser_request,omitempty"`
	BrowserObservedAt     int64                 `json:"browser_observed_at,omitempty"`
	Protocol              int                   `json:"protocol"`
	Revision              uint64                `json:"revision,omitempty"`
	Recording             bool                  `json:"recording"`
	Events                []Event               `json:"events,omitempty"`
	Outcome               string                `json:"outcome,omitempty"`
	Error                 string                `json:"error,omitempty"`
}

func readRPC(input io.Reader, value any) error {
	data, err := io.ReadAll(io.LimitReader(input, maxRPCBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxRPCBytes {
		return errors.New("recovery control message exceeds its limit")
	}
	if err := json.Unmarshal(data, value); err != nil {
		return errors.New("invalid recovery control message")
	}
	return nil
}

// Waiting sockets carry one newline-terminated JSON value in each direction.
// Both old and new writers use json.Encoder, so the delimiter is compatible
// with existing workers without depending on delivery of an AF_UNIX half-close.
// The stdio RPC above retains its whole-input, exactly-one-object contract.
func readWorkerRPC(input io.Reader, value any) error {
	data, err := bufio.NewReader(io.LimitReader(input, maxRPCBytes+1)).ReadBytes('\n')
	if len(data) > maxRPCBytes {
		return errors.New("recovery control message exceeds its limit")
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return errors.New("invalid recovery control message")
	}
	return nil
}

// ServeRPC deliberately processes exactly one bounded JSON object. Closing
// stdin completes a request; no remote shell evaluation carries its payload.
func ServeRPC(ctx context.Context, input io.Reader, output io.Writer) error {
	var request RPCRequest
	response := RPCResponse{Protocol: 1}
	if err := readRPC(input, &request); err != nil {
		response.Error = err.Error()
	} else {
		response = HostRPC(ctx, request)
	}
	return json.NewEncoder(output).Encode(response)
}

func HostRPC(ctx context.Context, request RPCRequest) RPCResponse {
	response, err := hostRPC(ctx, request)
	response.Protocol = 1
	if err != nil {
		response.Error = err.Error()
		if response.Outcome == "" {
			response.Outcome = "rejected"
		}
	}
	return response
}

func hostRPC(ctx context.Context, request RPCRequest) (RPCResponse, error) {
	if request.Protocol != 1 {
		return RPCResponse{}, errors.New("unsupported recovery control protocol")
	}
	// Provisioning is invoked only by the authenticated execution-host account.
	// It can create a new identity, but cannot replace a differently keyed one.
	if request.Operation == "ensure" {
		if request.Recording == nil {
			return RPCResponse{}, errors.New("missing recovery recording policy")
		}
		if err := InitializeHostEvidence(request.SessionID, request.Key, *request.Recording, request.Seed); err != nil {
			return RPCResponse{}, err
		}
		if request.BrowserControl {
			if _, err := updateHostState(request.SessionID, request.Key, false, func(state *HostState) error {
				state.BrowserControl = true
				return nil
			}); err != nil {
				return RPCResponse{}, err
			}
		}
		if _, err := EnsureProvider(ctx, request.SessionID, request.Key); err != nil {
			return RPCResponse{}, err
		}
		request.Operation = "snapshot"
	}
	if request.Operation == "coordination-prepare" {
		// Same authenticated execution-account provisioning boundary as ensure,
		// without starting a provider before its scoped channel is ready.
		if err := InitializeHostEvidence(request.SessionID, request.Key, false, nil); err != nil {
			return RPCResponse{}, err
		}
	}
	// Authenticate before returning state or connecting to a waiting worker.
	state, err := updateHostState(request.SessionID, request.Key, false, nil)
	if err != nil {
		return RPCResponse{}, err
	}
	switch request.Operation {
	case "restart-preview":
		selection, err := liveRestartSelection(ctx, request.SessionID)
		return RPCResponse{RestartSelection: selection}, err
	case "restart":
		return restartLiveProvider(ctx, request)
	case "coordination-observe":
		directory, err := coordinationProviderDirectory(ctx, request.SessionID)
		return RPCResponse{CoordinationDirectory: directory, CoordinationPlatform: runtime.GOOS}, err
	case "coordination-prepare":
		name, err := coordinationclient.PrepareSocket(request.SessionID, request.Key, request.CoordinationChannel)
		return RPCResponse{CoordinationSocket: name}, err
	case "coordination-install":
		if request.CoordinationAccess == nil {
			return RPCResponse{}, errors.New("missing scoped coordination access")
		}
		err := coordinationclient.InstallSocket(ctx, request.SessionID, request.Key, request.CoordinationChannel, *request.CoordinationAccess)
		if err != nil {
			return RPCResponse{}, err
		}
		return RPCResponse{Outcome: "coordination-installed"}, nil
	case "coordination-project":
		// Only the authenticated controller chooses this directory, from its
		// signed live shell observation. No agent-facing API accepts a path or
		// Git command. Inspect on this execution host, never on the controller.
		project, err := coordination.InspectRepository(ctx, request.CoordinationDirectory)
		return RPCResponse{CoordinationProject: project}, err
	case "snapshot":
	case "policy":
		if request.Recording == nil {
			return RPCResponse{}, errors.New("missing recovery recording policy")
		}
		state, err = updateHostState(request.SessionID, request.Key, false, func(state *HostState) error {
			state.Recording = *request.Recording
			if pending := state.BrowserRequest; pending != nil && (pending.ID == request.BrowserAck || pending.Expires <= time.Now().UnixMilli()) {
				state.BrowserRequest = nil
			}
			return nil
		})
	case "action":
		if request.Action == nil || !nativeUUID.MatchString(request.Action.ID) {
			return RPCResponse{}, errors.New("missing recovery action identity")
		}
		for _, consumed := range state.Consumed {
			if consumed == request.Action.ID {
				return RPCResponse{Outcome: "consumed"}, nil
			}
		}
		if state.Control == nil || state.Control.Mode != "waiting" || state.Control.Endpoint == "" || state.Control.Instance != request.Action.Instance {
			return RPCResponse{}, errors.New("this provider is not waiting for recovery; reconnect and refresh its state")
		}
		return callWaitingWorker(ctx, state.Control.Endpoint, request)
	default:
		return RPCResponse{}, errors.New("unknown recovery control operation")
	}
	if err != nil {
		return RPCResponse{}, err
	}
	events, err := SignedHostEvents(state, request.Key)
	var observedAt int64
	if state.BrowserRequest != nil {
		observedAt = time.Now().UnixMilli()
	}
	return RPCResponse{Revision: state.Revision, Recording: state.Recording, Events: events, BrowserRequest: state.BrowserRequest, BrowserObservedAt: observedAt}, err
}

func callWaitingWorker(ctx context.Context, endpoint string, request RPCRequest) (RPCResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, recoveryWorkerTimeout)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
	if err != nil {
		return RPCResponse{}, errors.New("the recovery worker is unavailable; reconnect to create a new waiting provider")
	}
	defer connection.Close()
	deadline, _ := ctx.Deadline()
	_ = connection.SetDeadline(deadline)
	// Cancellation can arrive after DialContext succeeds. Wake an established
	// socket read promptly too; a missing reply remains an uncertain outcome.
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.SetDeadline(time.Now()) })
	defer stopCancellation()
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return RPCResponse{Outcome: "uncertain"}, err
	}
	if err := connection.(*net.UnixConn).CloseWrite(); err != nil {
		return RPCResponse{Outcome: "uncertain"}, err
	}
	var response RPCResponse
	err = readWorkerRPC(connection, &response)
	if err != nil {
		response.Outcome = "uncertain"
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}
	return response, err
}
