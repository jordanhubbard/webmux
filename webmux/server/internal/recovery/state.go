package recovery

// Activity describes one observed command, not a reconstructed process image.
// NativeEvidence distinguishes the original selector from observed native state.
type Activity struct {
	Configuration   *NativeConfiguration `json:"configuration,omitempty" yaml:"configuration,omitempty"`
	Platform        string               `json:"platform,omitempty" yaml:"platform,omitempty"`
	Replayable      bool                 `json:"replayable" yaml:"replayable"`
	ShellInstance   string               `json:"shell_instance,omitempty" yaml:"shell_instance,omitempty"`
	Command         string               `json:"command,omitempty" yaml:"command,omitempty"`
	Argv            []string             `json:"argv,omitempty" yaml:"argv,omitempty"`
	Cwd             string               `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Source          string               `json:"source" yaml:"source"`
	ObservedAt      string               `json:"observed_at" yaml:"observed_at"`
	CLI             string               `json:"cli,omitempty" yaml:"cli,omitempty"`
	CLIVersion      string               `json:"cli_version,omitempty" yaml:"cli_version,omitempty"`
	NativeSessionID string               `json:"native_session_id,omitempty" yaml:"native_session_id,omitempty"`
	NativeStateFile string               `json:"native_state_file,omitempty" yaml:"native_state_file,omitempty"`
	NativeEvidence  string               `json:"native_evidence,omitempty" yaml:"native_evidence,omitempty"`
	Running         bool                 `json:"running" yaml:"running"`
}

func (a *Activity) NativeRecoverable() bool {
	return a != nil && ValidNativeID(a.CLI, a.NativeSessionID) && (a.Configuration == nil || a.Configuration.ManualReason == "")
}

// A launch selector establishes the conversation originally requested, not the
// conversation currently selected after an in-process switch. Older records
// without provenance likewise require the user's explicit choice.
func (a *Activity) CanAutoResumeNative() bool {
	return a.NativeRecoverable() && a.NativeEvidence == "process-state"
}

type State struct {
	Intent      *Intent   `json:"intent,omitempty" yaml:"intent,omitempty"`
	AutoResume  bool      `json:"auto_resume" yaml:"auto_resume"`
	Version     int       `json:"version" yaml:"version"`
	Revision    uint64    `json:"revision" yaml:"revision"`
	Activity    *Activity `json:"activity,omitempty" yaml:"activity,omitempty"`
	Transcripts []string  `json:"transcripts,omitempty" yaml:"transcripts,omitempty"`
	Pending     bool      `json:"pending" yaml:"pending"`
	Disposition string    `json:"disposition,omitempty" yaml:"disposition,omitempty"`
}

type Intent struct {
	ID      string `json:"id" yaml:"id"`
	Hash    string `json:"-" yaml:"hash"`
	Outcome string `json:"outcome" yaml:"outcome"`
}

func (s *State) Clone() *State {
	if s == nil {
		return nil
	}
	copy := *s
	if s.Intent != nil {
		intent := *s.Intent
		copy.Intent = &intent
	}
	copy.Transcripts = append([]string(nil), s.Transcripts...)
	if s.Activity != nil {
		a := *s.Activity
		a.Argv = append([]string(nil), a.Argv...)
		a.Configuration = a.Configuration.Clone()
		copy.Activity = &a
	}
	return &copy
}

func (s *State) AddTranscript(name string) {
	for _, previous := range s.Transcripts {
		if previous == name {
			return
		}
	}
	s.Transcripts = append(s.Transcripts, name)
	if len(s.Transcripts) > 8 {
		s.Transcripts = s.Transcripts[len(s.Transcripts)-8:]
	}
}
