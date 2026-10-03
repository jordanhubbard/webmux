//go:build windows

package browseropen

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func installOpeners(binary string) (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("%x", sha256.Sum256(data))[:16]
	dir := filepath.Join(cache, "webmux", "browser-open", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "webmux-open.exe")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = os.WriteFile(path, data, 0700)
		if err != nil {
			return "", err
		}
	}
	return dir, nil
}

func Shell() error {
	env, err := ShellEnvironment()
	if err != nil {
		return err
	}
	shell := os.Getenv("COMSPEC")
	if shell == "" {
		shell = "cmd.exe"
	}
	cmd := exec.Command(shell)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func Execute(path string, args, env []string) error {
	cmd := exec.Command(path, args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
