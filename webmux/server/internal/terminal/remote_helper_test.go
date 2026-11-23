//go:build !windows

package terminal

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectBrowserFallbackChild(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_CONNECT_FALLBACK") != "1" {
		return
	}
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(browserConnect{Request: testRequest(), Path: ssh, Args: []string{"fixture-login"}})
	if err == nil {
		err = ConnectBrowser(string(data))
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestMissingRecoveryHelperWarnsBeforeOrdinarySSHLogin(t *testing.T) {
	dir := t.TempDir()
	ssh := "#!/bin/sh\nif [ \"$1\" = fixture-login ]; then printf 'ORDINARY_LOGIN\\n'; exit 0; fi\nexit 127\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(ssh), 0700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^TestConnectBrowserFallbackChild$")
	cmd.Env = append(os.Environ(), "WEBMUX_TEST_CONNECT_FALLBACK=1", "WEBMUX_RECOVERY_KEY=fixture-key", "PATH="+dir+":/usr/bin:/bin")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "not protected against disconnects or WebMux restarts") || !strings.HasSuffix(string(output), "ORDINARY_LOGIN\n") {
		t.Fatalf("unprotected login was silent or unavailable: %q, %v", output, err)
	}
}

func TestRemoteHelperFindsInstalledRuntimeWithoutLoginProfile(t *testing.T) {
	for _, location := range []string{".local/bin/webmux", "Applications/WebMux.app/Contents/MacOS/webmux"} {
		t.Run(location, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "account with spaces")
			helper := filepath.Join(home, location)
			if err := os.MkdirAll(filepath.Dir(helper), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(helper, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\ncat\n"), 0700); err != nil {
				t.Fatal(err)
			}
			// A login profile must never run or contaminate authenticated RPC.
			if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("echo PROFILE_RAN; cat\n"), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("/bin/sh", "-c", remoteHelperCommand("--recovery-rpc", "literal ' ; $(echo INJECTED)"))
			cmd.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
			cmd.Stdin = strings.NewReader("private stdin payload\n")
			output, err := cmd.CombinedOutput()
			if err != nil || string(output) != "--recovery-rpc\nliteral ' ; $(echo INJECTED)\nprivate stdin payload\n" {
				t.Fatalf("helper invocation corrupted args or stdin: %q, %v", output, err)
			}
		})
	}
}

func TestRemoteHelperRetainsConfiguredPathPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "webmux"), []byte("#!/bin/sh\nprintf 'configured-helper'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", remoteHelperCommand("--recovery-capabilities"))
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + dir + ":/usr/bin:/bin"}
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "configured-helper" {
		t.Fatalf("configured helper lost precedence: %q, %v", output, err)
	}
}
