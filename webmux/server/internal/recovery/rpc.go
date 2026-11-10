package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"
)

const maxRPCBytes = 256 << 10

// RPCRequest travels through a local pipe or authenticated SSH stdin. Keys must
// never appear in a URL, command-line argument, terminal input or transcript.
type RPCRequest struct {
	Protocol  int             `json:"protocol"`
	SessionID string          `json:"session_id"`
	Key       string          `json:"key"`
	Operation string          `json:"operation"`
	Recording *bool           `json:"recording,omitempty"`
	Action    *RecoveryAction `json:"action,omitempty"`
	Seed      *Activity       `json:"seed,omitempty"`
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
	Protocol  int     `json:"protocol"`
	Revision  uint64  `json:"revision,omitempty"`
	Recording bool    `json:"recording"`
	Events    []Event `json:"events,omitempty"`
	Outcome   string  `json:"outcome,omitempty"`
	Error     string  `json:"error,omitempty"`
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
		if _, err := EnsureProvider(ctx, request.SessionID, request.Key); err != nil {
			return RPCResponse{}, err
		}
		request.Operation = "snapshot"
	}
	// Authenticate before returning state or connecting to a waiting worker.
	state, err := updateHostState(request.SessionID, request.Key, false, nil)
	if err != nil {
		return RPCResponse{}, err
	}
	switch request.Operation {
	case "snapshot":
	case "policy":
		if request.Recording == nil {
			return RPCResponse{}, errors.New("missing recovery recording policy")
		}
		state, err = updateHostState(request.SessionID, request.Key, false, func(state *HostState) error {
			state.Recording = *request.Recording
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
	return RPCResponse{Revision: state.Revision, Recording: state.Recording, Events: events}, err
}

func callWaitingWorker(ctx context.Context, endpoint string, request RPCRequest) (RPCResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
	if err != nil {
		return RPCResponse{}, errors.New("the recovery worker is unavailable; reconnect to create a new waiting provider")
	}
	defer connection.Close()
	deadline, _ := ctx.Deadline()
	_ = connection.SetDeadline(deadline)
	if err := json.NewEncoder(connection).Encode(request); err != nil {
		return RPCResponse{Outcome: "uncertain"}, err
	}
	if err := connection.(*net.UnixConn).CloseWrite(); err != nil {
		return RPCResponse{Outcome: "uncertain"}, err
	}
	var response RPCResponse
	err = readRPC(connection, &response)
	if err != nil {
		response.Outcome = "uncertain"
	}
	return response, err
}
