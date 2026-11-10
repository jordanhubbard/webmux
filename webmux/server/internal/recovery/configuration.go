package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// NativeConfiguration records paths, never credentials or arbitrary environment.
// A non-nil snapshot also records absence: unset roots must not inherit a
// different terminal's override when the provider is recreated.
type NativeConfiguration struct {
	Roots        map[string]string `json:"roots" yaml:"roots"`
	ManualReason string            `json:"manual_reason,omitempty" yaml:"manual_reason,omitempty"`
}

func (c *NativeConfiguration) Clone() *NativeConfiguration {
	if c == nil {
		return nil
	}
	return &NativeConfiguration{Roots: maps.Clone(c.Roots), ManualReason: c.ManualReason}
}

func nativeRootNames(cli string) []string {
	switch cli {
	case "codex":
		return []string{"HOME", "CODEX_HOME"}
	case "claude":
		return []string{"HOME", "CLAUDE_CONFIG_DIR"}
	case "cursor":
		return []string{"HOME", "CURSOR_CONFIG_DIR", "XDG_CONFIG_HOME"}
	case "opencode":
		return []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR"}
	}
	return nil
}

func captureNativeConfiguration(cli string) *NativeConfiguration {
	c := &NativeConfiguration{Roots: map[string]string{}}
	for _, name := range nativeRootNames(cli) {
		if value, exists := os.LookupEnv(name); exists {
			c.Roots[name] = value
		}
	}
	if cli == "opencode" && (os.Getenv("OPENCODE_CONFIG_CONTENT") != "" || os.Getenv("OPENCODE_PERMISSION") != "") {
		c.ManualReason = "This OpenCode invocation uses inline configuration or permission settings. Restore it manually; those settings are not recorded."
	}
	if err := c.Validate(cli); err != nil {
		return &NativeConfiguration{Roots: map[string]string{}, ManualReason: "The original coding CLI configuration paths could not be recorded. Restore this invocation manually."}
	}
	return c
}

// Persist a bounded explanation as soon as the original invocation is seen,
// so the preview never offers native recovery that would discard settings.
func captureNativeInvocationConfiguration(cli string, args []string) *NativeConfiguration {
	c := captureNativeConfiguration(cli)
	if c.ManualReason == "" {
		if _, err := nativeResumeOptions(cli, args); err != nil {
			c.ManualReason = err.Error()
		}
	}
	return c
}

func (c *NativeConfiguration) Validate(cli string) error {
	if c == nil {
		return nil
	}
	allowed := map[string]bool{}
	for _, name := range nativeRootNames(cli) {
		allowed[name] = true
	}
	if len(allowed) == 0 || len(c.Roots) > len(allowed) || len(c.ManualReason) > 512 || strings.ContainsFunc(c.ManualReason, unicode.IsControl) {
		return errors.New("invalid native configuration snapshot")
	}
	size := 0
	for name, value := range c.Roots {
		size += len(name) + len(value)
		if !allowed[name] || len(value) > 4096 || strings.ContainsFunc(value, unicode.IsControl) {
			return errors.New("invalid native configuration path")
		}
	}
	if size > 8192 {
		return errors.New("native configuration paths are too large")
	}
	return nil
}

func nativeEnvironment(cli string, c *NativeConfiguration, inherited []string) ([]string, error) {
	if c == nil {
		return append([]string(nil), inherited...), nil
	}
	if err := c.Validate(cli); err != nil {
		return nil, err
	}
	if c.ManualReason != "" {
		return nil, errors.New(c.ManualReason)
	}
	remove := map[string]bool{}
	for _, name := range nativeRootNames(cli) {
		remove[name] = true
	}
	// Never accidentally inherit an unrecorded inline policy from a recreated
	// provider. Such original invocations already require manual restoration.
	if cli == "opencode" {
		remove["OPENCODE_CONFIG_CONTENT"], remove["OPENCODE_PERMISSION"] = true, true
	}
	var environment []string
	for _, entry := range inherited {
		name, _, _ := strings.Cut(entry, "=")
		if !remove[name] {
			environment = append(environment, entry)
		}
	}
	for name, value := range c.Roots {
		environment = append(environment, name+"="+value)
	}
	return environment, nil
}

type nativePreflightResult struct {
	Argv  []string `json:"argv,omitempty"`
	Error string   `json:"error,omitempty"`
}

// A separate process gives preflight the original configuration roots without
// mutating the broker or waiting worker's environment/working directory.
func prepareNativeResume(request NativeResume) ([]string, error) {
	if request.Activity.Configuration == nil {
		return prepareNativeResumeCurrent(request)
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return preflightWithCommand(request, []string{self, "--recovery-preflight"})
}

func preflightWithCommand(request NativeResume, command []string) ([]string, error) {
	environment, err := nativeEnvironment(request.Activity.CLI, request.Activity.Configuration, os.Environ())
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(request)
	if err != nil || len(data) > 64<<10 {
		return nil, errors.New("native recovery metadata is too large")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = request.Activity.Cwd
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second
	for _, entry := range environment {
		if request.Activity.Cwd != "" && strings.HasPrefix(entry, "PWD=") {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	if request.Activity.Cwd != "" {
		cmd.Env = append(cmd.Env, "PWD="+request.Activity.Cwd)
	}
	cmd.Stdin = strings.NewReader(string(data))
	var output boundedCLIOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil || output.exceeded {
		return nil, errors.New("cannot verify native state with the original CLI configuration")
	}
	var result nativePreflightResult
	if json.Unmarshal(output.buffer.Bytes(), &result) != nil {
		return nil, errors.New("invalid native preflight response")
	}
	if result.Error != "" {
		return nil, errors.New(result.Error)
	}
	if len(result.Argv) == 0 {
		return nil, errors.New("native preflight returned no continuation")
	}
	return result.Argv, nil
}

func ServeNativePreflight(input io.Reader, output io.Writer) error {
	return ServeNativePreflightContext(context.Background(), input, output)
}

func ServeNativePreflightContext(ctx context.Context, input io.Reader, output io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(input, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return errors.New("native preflight request is too large")
	}
	var request NativeResume
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return errors.New("invalid native preflight request")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("native preflight accepts one request")
	}
	if err := request.Activity.Configuration.Validate(request.Activity.CLI); err != nil {
		return err
	}
	if request.Activity.Configuration != nil && request.Activity.Configuration.ManualReason != "" {
		return errors.New(request.Activity.Configuration.ManualReason)
	}
	args, err := prepareNativeResumeContext(ctx, request)
	result := nativePreflightResult{Argv: args}
	if err != nil {
		result.Argv, result.Error = nil, err.Error()
	}
	return json.NewEncoder(output).Encode(result)
}

// Called only in the dedicated execution helper after its control socket has
// closed and consumption is durable. Restore that helper's roots before RunCLI;
// the controller and parent login shell are separate processes. Keeping RunCLI
// in this process also preserves its terminal/signal ownership.
func RunConfiguredCLI(args []string, cli string, configuration *NativeConfiguration) error {
	if len(args) == 0 {
		return errors.New("native continuation is missing")
	}
	if configuration == nil {
		return RunCLI(args[0], args[1:])
	}
	if err := configuration.Validate(cli); err != nil {
		return err
	}
	if configuration.ManualReason != "" {
		return errors.New(configuration.ManualReason)
	}
	for _, name := range nativeRootNames(cli) {
		var err error
		if value, exists := configuration.Roots[name]; exists {
			err = os.Setenv(name, value)
		} else {
			err = os.Unsetenv(name)
		}
		if err != nil {
			return err
		}
	}
	if cli == "opencode" {
		_ = os.Unsetenv("OPENCODE_CONFIG_CONTENT")
		_ = os.Unsetenv("OPENCODE_PERMISSION")
	}
	return RunCLI(args[0], args[1:])
}
