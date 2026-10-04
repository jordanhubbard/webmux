//go:build !windows

package browseropen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartupSurvivesProfilePATHAndBrowserOverrides(t *testing.T) {
	for _, name := range []string{"bash", "zsh"} {
		t.Run(name, func(t *testing.T) {
			shell, err := exec.LookPath(name)
			if err != nil {
				t.Skip("shell unavailable")
			}
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("ZDOTDIR", home)
			t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
			profile := "export PATH=/usr/bin:/bin\nexport BROWSER=wrong\nexport GH_BROWSER=wrong\nexport USER_SETTING=preserved\n"
			for _, file := range []string{".bash_profile", ".zprofile", ".zshrc", ".zlogin"} {
				if err := os.WriteFile(filepath.Join(home, file), []byte(profile), 0600); err != nil {
					t.Fatal(err)
				}
			}
			env, err := ShellEnvironment()
			if err != nil {
				t.Fatal(err)
			}
			args, env, err := shellStartup(shell, env)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, "-c", `printf '%s\n' "$BROWSER" "$GH_BROWSER" "$USER_SETTING"; command -v open; command -v xdg-open`)
			cmd := exec.Command(shell, args[1:]...)
			cmd.Env = env
			output, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			if len(lines) != 5 || !strings.HasSuffix(lines[0], "/webmux-open") || lines[0] != lines[1] || lines[2] != "preserved" || filepath.Dir(lines[3]) != filepath.Dir(lines[0]) || filepath.Dir(lines[4]) != filepath.Dir(lines[0]) {
				t.Fatalf("startup lost handoff or user settings: %q", output)
			}
		})
	}
}
