//go:build darwin

package maccoord

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

// This optional control consumes an explicitly supplied native credential in place.
// It never enrolls/renews/revokes a principal or claims least-privilege read scope.
func TestNativeMACRegistrationRead(t *testing.T) {
	token := os.Getenv("WEBMUX_MAC_REGISTRATION_NATIVE_TOKEN")
	if token == "" {
		t.Skip("requires an explicit native credential and pinned publisher")
	}
	origin := os.Getenv("WEBMUX_MAC_REGISTRATION_NATIVE_ORIGIN")
	project := os.Getenv("WEBMUX_MAC_REGISTRATION_NATIVE_PROJECT")
	pid, err := strconv.Atoi(os.Getenv("WEBMUX_MAC_REGISTRATION_NATIVE_PID"))
	u, urlErr := url.Parse(origin)
	if err != nil || pid < 1 || urlErr != nil || !validOrigin(origin) || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || !validReadID(project) {
		t.Fatal("explicit loopback publisher and exact project selection required")
	}
	publisher := func() string {
		t.Helper()
		output, err := exec.CommandContext(t.Context(), "lsof", "-nP", "-iTCP:"+u.Port(), "-sTCP:LISTEN", "-Fpu").Output()
		if err != nil {
			t.Fatal("native publisher inventory unavailable")
		}
		seenPID, seenUID := false, false
		for _, line := range strings.Split(string(output), "\n") {
			if strings.HasPrefix(line, "p") {
				if line != "p"+strconv.Itoa(pid) {
					t.Fatal("unexpected publisher process")
				}
				seenPID = true
			}
			if strings.HasPrefix(line, "u") {
				if line != "u"+strconv.Itoa(os.Getuid()) {
					t.Fatal("publisher OS account mismatch")
				}
				seenUID = true
			}
		}
		if !seenPID || !seenUID {
			t.Fatal("native publisher identity missing")
		}
		start, err := exec.CommandContext(t.Context(), "ps", "-p", strconv.Itoa(pid), "-o", "pid=,uid=,lstart=,comm=").Output()
		if err != nil || len(start) == 0 {
			t.Fatal("native publisher start identity unavailable")
		}
		digest := sha256.Sum256(start)
		return hex.EncodeToString(digest[:])
	}
	before := publisher()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal("native source context unavailable")
	}
	repo, err := coordination.InspectRepository(t.Context(), cwd)
	if err != nil {
		t.Fatal("native repository witnesses unavailable")
	}
	doc := bindingDocument{Schema: BindingsSchema, Authorities: []Authority{{ID: "native-operator-registration-proof", Origin: origin}}, Bindings: []ProjectBinding{{Owner: "native-qualification", RepositoryID: repo.ID, Repository: repo.Repository, AuthorityID: "native-operator-registration-proof", ProjectKey: project, Capabilities: []string{ObserveCapability}}}}
	b, err := ParseBindings(bindingBytes(t, doc))
	if err != nil {
		t.Fatal("explicit test-only binding refused")
	}
	got, err := b.ObserveProjectRegistration(t.Context(), "native-qualification", repo, token, nil)
	refusal := os.Getenv("WEBMUX_MAC_REGISTRATION_NATIVE_EXPECT_REFUSAL") == "1"
	if refusal {
		if !errors.Is(err, ErrReadAccess) || got.Schema != "" || got.ProjectID != "" {
			t.Fatal("revoked native credential returned registration evidence", err)
		}
	} else if err != nil || got.Schema != RegistrationObservationSchema || got.ProjectKey != project || got.ProjectID == "" || got.RepositoryLinkState != "unverified" || got.ReadAuthentication.PrincipalIdentity != "unverified" {
		t.Fatal("native canonical registration read failed", err)
	}
	foreign, err := b.ObserveProjectRegistration(t.Context(), "foreign-account", repo, token, nil)
	if !errors.Is(err, ErrUnmapped) || foreign.Schema != "" {
		t.Fatal("foreign receiving account crossed the local binding")
	}
	if publisher() != before {
		t.Fatal("native publisher changed during observation")
	}
	receipt := struct {
		Schema                string                  `json:"schema"`
		ObservedAt            time.Time               `json:"observed_at"`
		PublisherFingerprint  string                  `json:"publisher_fingerprint"`
		Observation           RegistrationObservation `json:"observation"`
		ForeignAccountRefused bool                    `json:"foreign_account_refused"`
		ExpectedRefusal       bool                    `json:"expected_refusal"`
		Scope                 string                  `json:"scope"`
	}{"webmux.mac.native-registration-proof.v1", time.Now().UTC(), before, got, true, refusal, "Explicit credential loopback registration read only; test binding does not enroll WebMux or prove least-privilege scope, actual revocation, repository registration, UI/agent integration or three-host acceptance"}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("native receipt serialization failed")
	}
	if strings.Contains(string(encoded), token) {
		t.Fatal("native token escaped projection")
	}
	t.Log(string(encoded))
}
