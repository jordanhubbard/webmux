package recovery

// Activity describes one observed command, not a reconstructed process image.
// NativeSessionID is only set by a provider adapter using its native state.
type Activity struct {
	Platform        string   `json:"platform,omitempty" yaml:"platform,omitempty"`
	Replayable      bool     `json:"replayable" yaml:"replayable"`
	ShellInstance   string   `json:"shell_instance,omitempty" yaml:"shell_instance,omitempty"`
	Command         string   `json:"command,omitempty" yaml:"command,omitempty"`
	Argv            []string `json:"argv,omitempty" yaml:"argv,omitempty"`
	Cwd             string   `json:"cwd,omitempty" yaml:"cwd,omitempty"`
	Source          string   `json:"source" yaml:"source"`
	ObservedAt      string   `json:"observed_at" yaml:"observed_at"`
	CLI             string   `json:"cli,omitempty" yaml:"cli,omitempty"`
	CLIVersion      string   `json:"cli_version,omitempty" yaml:"cli_version,omitempty"`
	NativeSessionID string   `json:"native_session_id,omitempty" yaml:"native_session_id,omitempty"`
	NativeStateFile string   `json:"native_state_file,omitempty" yaml:"native_state_file,omitempty"`
	Running         bool     `json:"running" yaml:"running"`
}

type State struct {
	AutoResume  bool      `json:"auto_resume" yaml:"auto_resume"`
	Version     int       `json:"version" yaml:"version"`
	Revision    uint64    `json:"revision" yaml:"revision"`
	Activity    *Activity `json:"activity,omitempty" yaml:"activity,omitempty"`
	Transcripts []string  `json:"transcripts,omitempty" yaml:"transcripts,omitempty"`
	Pending     bool      `json:"pending" yaml:"pending"`
	Disposition string    `json:"disposition,omitempty" yaml:"disposition,omitempty"`
}

func (s *State) Clone() *State {
	if s == nil {
		return nil
	}
	copy := *s
	copy.Transcripts = append([]string(nil), s.Transcripts...)
	if s.Activity != nil {
		a := *s.Activity
		a.Argv = append([]string(nil), a.Argv...)
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
