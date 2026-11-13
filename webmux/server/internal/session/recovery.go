package session

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/recovery"
)

// RecoveryView is owner-scoped like the terminal itself. History is plain text;
// browser/clipboard/shell protocol packets are never replayed from disk.
type RecoveryView struct {
	ManagerSnapshotID       string          `json:"manager_snapshot_id,omitempty"`
	CanResumeManaged        bool            `json:"can_resume_managed"`
	ManagerProfile          string          `json:"manager_profile,omitempty"`
	ManagerNativeSession    string          `json:"manager_native_session,omitempty"`
	ManagerNativeStateFile  string          `json:"manager_native_state_file,omitempty"`
	ManagerNativeSource     string          `json:"manager_native_source,omitempty"`
	ManagerNativeObservedAt string          `json:"manager_native_observed_at,omitempty"`
	ManagerTarget           string          `json:"manager_target,omitempty"`
	ManagerDirectory        string          `json:"manager_directory,omitempty"`
	Waiting                 bool            `json:"waiting"`
	HostRevision            uint64          `json:"host_revision"`
	Instance                string          `json:"instance"`
	Warning                 string          `json:"warning,omitempty"`
	CanResumeCLI            bool            `json:"can_resume_cli"`
	CanReplay               bool            `json:"can_replay"`
	SessionID               string          `json:"session_id"`
	Hostname                string          `json:"hostname"`
	Transport               string          `json:"transport"`
	LifecycleOwner          string          `json:"lifecycle_owner"`
	State                   *recovery.State `json:"state,omitempty"`
	History                 string          `json:"history"`
	HistoryAvailable        bool            `json:"history_available"`
	ShellReady              bool            `json:"shell_ready"`
	Message                 string          `json:"message"`
}

func (b *Broker) Recovery(owner, id string) (RecoveryView, error) {
	b.mu.Lock()
	e, err := b.ownedLocked(owner, id)
	if err != nil {
		b.mu.Unlock()
		return RecoveryView{}, err
	}
	s := e.value.clone()
	ready := e.shellReady
	helper, platform := e.shellHelper, e.shellPlatform
	var control *recovery.Event
	var hostRevision uint64
	var controlError string
	var hostReady bool
	var warning string
	live := e.run != nil
	if e.run != nil {
		control = e.run.recoveryControl
		hostReady = e.run.recoveryReady
		warning = e.run.recoveryWarning
		hostRevision, controlError = e.run.recoveryRevision, e.run.recoveryError
	}
	b.mu.Unlock()
	view := RecoveryView{SessionID: id, Hostname: s.Hostname, Transport: s.Transport, LifecycleOwner: "webmux", State: s.Recovery, ShellReady: ready, Message: "No recorded activity is available. Reconnect or recover this session manually."}
	view.HostRevision = hostRevision
	view.Warning = warning
	if control != nil {
		view.Waiting, view.Instance = control.Mode == "waiting" && hostReady, control.Instance
	}
	if s.Rig != nil {
		view.Instance = managedInstance(*s.Rig)
		view.CanResumeManaged = s.Recovery != nil && managedNativeSupported(*s.Rig)
		view.ManagerProfile = s.Rig.ManagerProfile
		view.ManagerSnapshotID = s.Rig.ManagerSnapshotID
		view.ManagerNativeSession, view.ManagerNativeStateFile = s.Rig.NativeSessionID, s.Rig.NativeStateFile
		view.ManagerNativeSource, view.ManagerNativeObservedAt = s.Rig.NativeSource, s.Rig.NativeObservedAt
		view.ManagerTarget, view.ManagerDirectory = s.Rig.Target, s.Rig.Workspace
		if s.Rig.Host != "" {
			view.Hostname = s.Rig.Host
		}
		view.LifecycleOwner = s.Rig.Provider
		if view.LifecycleOwner == "" {
			view.LifecycleOwner = "rig"
		}
	}
	if s.Agent() {
		view.LifecycleOwner = "agent-deck"
	}
	if s.Recovery != nil && s.Recovery.Activity != nil && view.Waiting && view.LifecycleOwner == "webmux" {
		a := s.Recovery.Activity
		view.CanResumeCLI = helper != "" && a.NativeRecoverable()
		view.CanReplay = a.Replayable && (platform == "linux" || platform == "darwin")
	}
	if s.Recovery == nil {
		if controlError != "" {
			view.Message = controlError
		}
		return view, nil
	}
	var histories []string
	for _, name := range s.Recovery.Transcripts {
		text, err := recovery.History(filepath.Join(b.store.Home, "logs", "sessions"), name)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				b.logger.Warn("read recovery history", "session_id", id, "error", err)
			}
			continue
		}
		if text != "" {
			histories = append(histories, text)
		}
	}
	view.History = strings.Join(histories, "\r\n")
	if len(view.History) > recovery.MaxHistoryBytes {
		view.History = strings.ToValidUTF8(view.History[len(view.History)-recovery.MaxHistoryBytes:], "")
	}
	view.HistoryAvailable = view.History != ""
	if s.Recovery.Activity != nil {
		view.Message = "Review the recorded activity and available history before continuing."
	} else if view.HistoryAvailable {
		view.Message = "Previous terminal output is available; no reliable command was recorded."
	}
	if s.Recovery.Disposition == "reattached" {
		view.Message = "Reattached to the surviving session. Its running work was preserved."
		if !live {
			// The disposition records the last restore; the viewer can end later.
			view.Message = "This session was reattached, but its terminal has since ended. Retained terminal history remains available; reconnect to check whether the original process is still running."
		}
	}
	if s.Recovery.Disposition == "managed-unavailable" {
		view.Message = "The original managed session could not be reattached. Retained terminal history remains available. " + s.Recovery.ManagerMessage
	}
	if view.Waiting {
		view.Message = "The previous provider was interrupted. Review the recorded work, then choose how to continue."
	}
	if s.Recovery.Disposition == "execution-uncertain" {
		view.Message = "The previous recovery request may have started before the connection was lost. Refresh the host state before choosing another action."
	}
	if controlError != "" {
		view.Message += " " + controlError
	}
	if a := s.Recovery.Activity; a != nil && a.Configuration != nil && a.Configuration.ManualReason != "" {
		view.Message += " " + a.Configuration.ManualReason
	}
	return view, nil
}

func (b *Broker) recoveryEventLocked(e *entry, r *run, event recovery.Event) {
	if !recovery.Verify(event, e.value.RecoveryKey) {
		return
	}
	if event.Kind == "shell" {
		if r.recoveryControl != nil && r.recoveryControl.Instance == event.Instance {
			copy := *r.recoveryControl
			copy.Mode = "active"
			copy.Ready = true
			r.recoveryControl = &copy
		}
		e.shellInstance, e.shellReady = event.Instance, true
		e.shellHelper, e.shellPlatform = event.Helper, event.Platform
		b.restoreRecoveryHistoryLocked(e, r)
		return
	}
	state := e.value.Recovery
	if event.Kind == "provider" {
		e.shellHelper, e.shellPlatform = event.Helper, event.Platform
		if event.Platform == "windows" {
			// A prior consumed action can leave Pending false. The new
			// provider's waiting state is authoritative before its image.
			if event.Mode == "waiting" {
				if e.value.Recovery == nil {
					e.value.Recovery = &recovery.State{Version: 1}
				}
				e.value.Recovery.Pending = true
				state = e.value.Recovery
			}
			// The native viewer's fresh provider event precedes its screen
			// snapshot. Put old history in scrollback before that image, not
			// later when the asynchronous control-state poll completes.
			b.markConnectedLocked(e.value.ID, e, r)
			if !event.Survived || event.Mode == "waiting" {
				b.restoreRecoveryHistoryLocked(e, r)
			}
		}
		b.startRecoveryPollingLocked(e, r)
		if state != nil && state.Pending && event.Survived && event.Mode != "waiting" {
			state.Pending, state.Disposition = false, "reattached"
			state.Revision++
			if err := b.persistLocked(); err != nil {
				b.logger.Warn("save recovery reattachment", "error", err)
			}
		}
		return
	}
	if event.Kind == "control" {
		if r.recoveryControl == nil || r.recoveryControl.Mode != event.Mode || r.recoveryControl.Instance != event.Instance {
			r.recoveryReady = false
		}
		copy := event
		r.recoveryControl = &copy
		if event.Mode == "waiting" {
			e.shellReady = false
			b.restoreRecoveryHistoryLocked(e, r)
		}
		return
	}
	switch event.Kind {
	case "command", "cli":
		e.shellReady = false
	case "complete":
		e.shellReady = true
	}
	// Honor both global defaults and the per-session pause switch. Applications
	// cannot enable recording simply by printing a metadata packet.
	if !r.log.active() {
		return
	}
	if state == nil {
		state = &recovery.State{Version: 1}
		e.value.Recovery = state
	}
	previous := state.Clone()
	switch event.Kind {
	case "command":
		if event.Command == "" {
			return
		}
		e.shellReady = false
		command, replayable := recovery.SafeCommand(event.Command)
		replayable = replayable && !event.Redacted
		state.Activity = &recovery.Activity{Platform: event.Platform, Replayable: replayable, ShellInstance: event.Instance, Command: command, Cwd: event.Cwd, Source: "shell-integration", ObservedAt: now(), Running: true}
		state.Pending, state.Disposition = false, "active"
	case "cli":
		e.shellReady = false
		a := state.Activity
		if a == nil {
			a = &recovery.Activity{}
			state.Activity = a
		}
		a.CLI, a.CLIVersion, a.NativeSessionID, a.NativeStateFile = event.CLI, event.Version, event.SessionID, event.StateFile
		a.NativeEvidence = event.NativeEvidence
		a.Configuration = event.Configuration.Clone()
		a.ShellInstance = event.Instance
		a.Platform = event.Platform
		a.Cwd, a.Argv, a.Source, a.Running = event.Cwd, append([]string(nil), event.Argv...), "cli-native-state", true
		if a.Command == "" {
			a.Command = strings.Join(event.Argv, " ")
		}
		state.Pending, state.Disposition = false, "active"
	case "complete":
		e.shellReady = true
		if state.Activity != nil && state.Activity.ShellInstance != event.Instance {
			return
		}
		if state.Activity != nil {
			state.Activity.Running = false
		}
	default:
		return
	}
	if reflect.DeepEqual(previous, state) {
		return
	}
	state.Revision++
	if state.Activity != nil {
		state.Activity.ObservedAt = now()
	}
	if err := b.persistLocked(); err != nil {
		b.logger.Warn("save recovery activity", "session_id", e.value.ID, "error", err)
	}
	b.broadcastLocked(e, Event{"type": "recovery_status", "session_id": e.value.ID, "recovery": state.Clone()})
}

func (b *Broker) restoreRecoveryHistoryLocked(e *entry, r *run) {
	if e.historyRestored || e.value.Recovery == nil || !e.value.Recovery.Pending {
		return
	}
	e.historyRestored = true
	for i := len(e.value.Recovery.Transcripts) - 1; i >= 0; i-- {
		name := e.value.Recovery.Transcripts[i]
		if r.log != nil && r.log.name == name {
			continue
		}
		text, err := recovery.History(filepath.Join(b.store.Home, "logs", "sessions"), name)
		if err != nil || text == "" {
			continue
		}
		text = strings.ReplaceAll(text, "\n", "\r\n")
		text = "\r\n[Previous recorded output — review Recover work to continue]\r\n" + text + "\r\n[Current terminal]\r\n"
		if e.shellPlatform == "windows" {
			// Native console coordinates start at the viewport's top. Move
			// the complete plain history above it; the next native snapshot
			// paints the viewport and restores modes/cursor before live bytes.
			text = "\x1b[?1049l\x1b[?6l\x1b[r\x1b[0m\x1b[?7h" + text + strings.Repeat("\r\n", min(max(e.value.Rows, 1), 200)) + "\x1b[H"
		}
		e.appendReplay(text)
		b.broadcastLocked(e, Event{"type": "output", "session_id": e.value.ID, "data": text})
		break
	}
}
