package browser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSavedProfileLockAndScopedRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	key := strings.Repeat("a", 64)
	path, lock, err := savedProfile(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, other, err := savedProfile(key); err == nil {
		other.Close()
		t.Fatal("concurrent profile opened")
	}
	marker := filepath.Join(path, "cookie-marker")
	if err := os.WriteFile(marker, []byte("saved"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	data, _ := json.Marshal(Request{ProfileKey: key})
	if err := RemoveProfile(bytes.NewReader(data), &output); err == nil {
		t.Fatal("removed active profile")
	}
	lock.Close()
	reopened, lock, err := savedProfile(key)
	if err != nil {
		t.Fatal(err)
	}
	if reopened != path {
		t.Fatal("changed profile identity")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("profile not retained", err)
	}
	lock.Close()
	output.Reset()
	if err := RemoveProfile(bytes.NewReader(data), &output); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("profile not removed", err)
	}
	// Keep the lock inode: unlinking it could admit another owner of a still-open inode.
	if _, err := os.Stat(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"../escape", strings.Repeat("b", 63), strings.Repeat("A", 64), ""} {
		if _, lock, err := savedProfile(bad); err == nil {
			lock.Close()
			t.Fatal("accepted unsafe identity", bad)
		}
	}
}
func TestSavedProfileRejectsLinksAndPublicDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlink and mode boundary")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "webmux-browser-profiles")
	outside := t.TempDir()
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("b", 64)
	if _, lock, err := savedProfile(key); err == nil {
		lock.Close()
		t.Fatal("followed root link")
	}
	os.Remove(root)
	os.Mkdir(root, 0700)
	os.Symlink(outside, filepath.Join(root, key))
	if _, lock, err := savedProfile(key); err == nil {
		lock.Close()
		t.Fatal("followed profile link")
	}
	os.Remove(filepath.Join(root, key))
	os.Chmod(root, 0755)
	if _, lock, err := savedProfile(key); err == nil {
		lock.Close()
		t.Fatal("accepted public profile root")
	}
}

func TestProfileLockSurvivesOwnerCrash(t *testing.T) {
	key := strings.Repeat("e", 64)
	if os.Getenv("WEBMUX_PROFILE_LOCK_HELPER") == "1" {
		_, lock, err := savedProfile(key)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		fmt.Println("profile-locked")
		time.Sleep(time.Hour)
		return
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestProfileLockSurvivesOwnerCrash$")
	child.Env = append(os.Environ(), "WEBMUX_PROFILE_LOCK_HELPER=1")
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Process.Kill(); child.Wait() })
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(output)
		ready <- scanner.Scan() && scanner.Text() == "profile-locked"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("helper did not acquire profile")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper startup timeout")
	}
	if _, lock, err := savedProfile(key); err == nil {
		lock.Close()
		t.Fatal("another process opened locked profile")
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	child.Wait()
	_, lock, err := savedProfile(key)
	if err != nil {
		t.Fatal("kernel did not release crashed owner lock", err)
	}
	lock.Close()
}
