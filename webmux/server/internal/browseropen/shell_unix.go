//go:build !windows

package browseropen

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func installOpeners(binary string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("%x", sha256.Sum256([]byte(binary)))[:16]
	dir := filepath.Join(cache, "webmux", "browser-open", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	for _, name := range []string{"webmux-open", "open", "xdg-open", "sensible-browser"} {
		path := filepath.Join(dir, name)
		if err := os.Symlink(binary, path); err != nil {
			if target, readErr := os.Readlink(path); readErr != nil || target != binary {
				return "", err
			}
		}
	}
	return dir, nil
}

func Shell() error {
	env, err := ShellEnvironment()
	if err != nil {
		return err
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	shell, err = exec.LookPath(shell)
	if err != nil {
		return err
	}
	args, env, err := shellStartup(shell, env)
	if err != nil {
		return err
	}
	return Execute(shell, args, env)
}

func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }

// Login profiles frequently replace PATH. Apply shell-scoped integration after
// loading those profiles, without editing any user startup files.
func shellStartup(shell string, env []string) ([]string, []string, error) {
	bin := ""
	for _, item := range env {
		if value, ok := strings.CutPrefix(item, "WEBMUX_BROWSER_BIN="); ok {
			bin = value
		}
	}
	setup := "\nexport WEBMUX_BROWSER_ORIGINAL_PATH=\"$PATH\"\nexport PATH=" + quote(bin) + ":\"$PATH\"\nexport BROWSER=" + quote(filepath.Join(bin, "webmux-open")) + "\nexport GH_BROWSER=\"$BROWSER\"\n"
	write := func(path, data string) error {
		f, err := os.CreateTemp(filepath.Dir(path), ".startup-")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if _, err := f.WriteString(data); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return os.Rename(f.Name(), path)
	}
	switch filepath.Base(shell) {
	case "bash":
		rc := filepath.Join(bin, "bashrc")
		body := `# WebMux shell-scoped startup: source the usual login configuration.
if [ -r /etc/profile ]; then . /etc/profile; fi
if [ -r "$HOME/.bash_profile" ]; then . "$HOME/.bash_profile"
elif [ -r "$HOME/.bash_login" ]; then . "$HOME/.bash_login"
elif [ -r "$HOME/.profile" ]; then . "$HOME/.profile"; fi
`
		if err := write(rc, body+setup); err != nil {
			return nil, nil, err
		}
		return []string{shell, "--rcfile", rc, "-i"}, env, nil
	case "zsh":
		original := os.Getenv("ZDOTDIR")
		if original == "" {
			original = os.Getenv("HOME")
		}
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(original)))[:16]
		dir := filepath.Join(bin, "zsh-"+id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, nil, err
		}
		for _, name := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"} {
			file := quote(filepath.Join(original, name))
			body := "ZDOTDIR=" + quote(original) + "\nif [ -r " + file + " ]; then . " + file + "; fi\n"
			if name == ".zshrc" || name == ".zlogin" {
				body += setup
			}
			next := dir
			if name == ".zlogin" {
				next = original
			}
			body += "export ZDOTDIR=" + quote(next) + "\n"
			if err := write(filepath.Join(dir, name), body); err != nil {
				return nil, nil, err
			}
		}
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], "ZDOTDIR=") {
				env = append(env[:i], env[i+1:]...)
			}
		}
		return []string{shell, "-l"}, append(env, "ZDOTDIR="+dir), nil
	default:
		// Other shells still receive absolute BROWSER/GH_BROWSER paths.
		return []string{shell, "-l"}, env, nil
	}
}

func Execute(path string, args, env []string) error { return syscall.Exec(path, args, env) }
