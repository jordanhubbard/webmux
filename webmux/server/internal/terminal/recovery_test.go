package terminal

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoveryChannelUsesFixedSSHHelperWithNoCredentialPayloadInArguments(t *testing.T) {
	request := LaunchRequest{Transport: "mosh", Hostname: "example.test", Username: "worker", Port: 2222, RecoveryKey: "private-per-terminal-key"}
	args, err := recoverySSHArgs(request, "/private/control-socket")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "BatchMode=yes") || !strings.Contains(joined, "StrictHostKeyChecking=yes") || !strings.Contains(joined, "-p 2222") || !strings.Contains(joined, "ControlPath=/private/control-socket") || args[len(args)-1] != "webmux --recovery-rpc" || strings.Contains(joined, request.RecoveryKey) {
		t.Fatal("incorrect execution-host control channel", args)
	}
	// The browser-connect child gets its key through its inherited environment,
	// never in the serialized argv payload used for helper capability probing.
	data, err := json.Marshal(browserConnect{Request: request})
	if err != nil || strings.Contains(string(data), request.RecoveryKey) || strings.Contains(string(data), "RecoveryKey") {
		t.Fatal("recovery credential entered a launch argument", err)
	}
}

func TestRecoveryChannelRejectsUnownedTransportsAndInvalidDestinations(t *testing.T) {
	for _, request := range []LaunchRequest{
		{Transport: "exec", Hostname: "example.test"},
		{Transport: "ssh", Hostname: "-oProxyCommand=bad"},
		{Transport: "ssh", Hostname: "example.test", Username: "worker;bad"},
		{Transport: "ssh", Hostname: "example.test", Port: -1},
	} {
		if _, err := recoverySSHArgs(request, ""); err == nil {
			t.Fatal("accepted invalid recovery destination", request)
		}
	}
}
