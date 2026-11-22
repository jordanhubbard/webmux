package terminal

import (
	"reflect"
	"strings"
	"testing"
)

func TestCoordinationBootstrapPreservesTheOriginalHostTrustPolicy(t *testing.T) {
	strict := []string{"-o", "StrictHostKeyChecking=yes", "--", "host", "webmux --recovery-capabilities"}
	if got := recoveryBootstrapTrust([]string{"-o", "StrictHostKeyChecking=yes"}, strict); !reflect.DeepEqual(got, strict) {
		t.Fatal("strict original connection was weakened")
	}
	first := recoveryBootstrapTrust([]string{"-o", "StrictHostKeyChecking=accept-new"}, strict)
	if !strings.Contains(strings.Join(first, " "), "StrictHostKeyChecking=accept-new") || strict[1] != "StrictHostKeyChecking=yes" {
		t.Fatal("bootstrap did not preserve selected first-use policy or mutated background RPC options")
	}
}

func TestCoordinationForwardIsPrivateAndHasNoRemoteCommandOrCredentials(t *testing.T) {
	request := LaunchRequest{Transport: "ssh", Hostname: "execution.example.test", Username: "worker", Port: 2222, RecoveryKey: "PRIVATE_KEY_NEVER_ARGV"}
	args, err := coordinationForwardArgs(request, "/private/control", "http://127.0.0.1:12345", "/private/cc/s", "forward")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-O forward -R /private/cc/s:127.0.0.1:12345 -- execution.example.test") || strings.Contains(joined, request.RecoveryKey) || strings.Contains(joined, "--recovery-rpc") || !strings.Contains(joined, "StrictHostKeyChecking=yes") {
		t.Fatal("incorrect private SSH channel", args)
	}
	for _, bad := range []struct{ endpoint, socket, operation string }{
		{"http://public.example:12345", "/private/s", "forward"},
		{"https://127.0.0.1:12345", "/private/s", "forward"},
		{"http://127.0.0.1:0", "/private/s", "forward"},
		{"http://127.0.0.1:65536", "/private/s", "forward"},
		{"http://127.0.0.1:12345", "relative", "forward"},
		{"http://127.0.0.1:12345", "/private/s:22", "forward"},
		{"http://127.0.0.1:12345", "/private/s", "exit"},
	} {
		if _, err := coordinationForwardArgs(request, "/private/control", bad.endpoint, bad.socket, bad.operation); err == nil {
			t.Fatal("unsafe forward accepted", bad)
		}
	}
}
