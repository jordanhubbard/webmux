//go:build darwin

package maccoord

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

// Opt-in native qualification. Never enrolls, renews, revokes, dispatches or
// discovers a default profile. The operator supplies an own read-only enrollment
// manifest and the expected already-running local MAC publisher explicitly.
func TestNativeLocalMACReadAccess(t *testing.T) {
	path := os.Getenv("WEBMUX_MAC_NATIVE_MANIFEST")
	if path == "" {
		t.Skip("requires an explicitly supplied private read-only MAC enrollment")
	}
	origin := os.Getenv("WEBMUX_MAC_NATIVE_ORIGIN")
	pid, err := strconv.Atoi(os.Getenv("WEBMUX_MAC_NATIVE_PUBLISHER_PID"))
	if err != nil || pid < 1 || !validOrigin(origin) {
		t.Fatal("explicit native authority and publisher are required")
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" {
		t.Fatal("this native fixture qualifies a pinned IPv4-loopback MAC publisher only")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
		t.Fatal("native enrollment must be a bounded private regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil || !uniqueJSON(data) {
		t.Fatal("could not read the explicit native enrollment")
	}
	var manifest struct {
		Schema     string `json:"schema"`
		ClientID   string `json:"client_id"`
		Connection struct {
			APIURL string `json:"api_url"`
		} `json:"connection"`
		Credential struct {
			Token     string   `json:"token"`
			Scopes    []string `json:"scopes"`
			ExpiresAt string   `json:"expires_at"`
		} `json:"credential"`
	}
	if json.Unmarshal(data, &manifest) != nil || manifest.Schema != "mac.client_enrollment.v1" ||
		!strings.HasPrefix(manifest.ClientID, "webmux-") || manifest.Connection.APIURL != origin ||
		len(manifest.Credential.Scopes) != 1 || manifest.Credential.Scopes[0] != "read" {
		t.Fatal("native enrollment does not identify the intended dedicated read-only client")
	}
	expires, err := time.Parse(time.RFC3339Nano, manifest.Credential.ExpiresAt)
	if err != nil || !expires.After(time.Now()) {
		t.Fatal("native enrollment has expired or has no valid expiry")
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
					t.Fatal("another process owns the intended native port")
				}
				seenPID = true
			}
			if strings.HasPrefix(line, "u") {
				if line != "u"+strconv.Itoa(os.Getuid()) {
					t.Fatal("native publisher belongs to a different OS account")
				}
				seenUID = true
			}
		}
		if !seenPID || !seenUID {
			t.Fatal("native publisher identity missing")
		}
		start, err := exec.CommandContext(t.Context(), "ps", "-p", strconv.Itoa(pid), "-o", "pid=,uid=,lstart=,comm=").Output()
		if err != nil || len(start) == 0 {
			t.Fatal("native publisher process identity unavailable")
		}
		digest := sha256.Sum256(start)
		return hex.EncodeToString(digest[:])
	}
	before := publisher()
	projectKey, taskID := "webmux-auth-qualification", ""
	if selectionPath := os.Getenv("WEBMUX_MAC_NATIVE_TASK_SELECTION"); selectionPath != "" {
		info, err := os.Lstat(selectionPath)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 64<<10 {
			t.Fatal("native task selection must be a bounded private regular file")
		}
		data, err := os.ReadFile(selectionPath)
		var selection struct {
			Schema     string `json:"schema"`
			TaskID     string `json:"task_id"`
			ProjectKey string `json:"project_key"`
			Eligible   bool   `json:"test_only_mapping_eligible"`
		}
		if err != nil || !uniqueJSON(data) || json.Unmarshal(data, &selection) != nil ||
			selection.Schema != TaskSchema || !selection.Eligible || !validText(selection.TaskID, 256) {
			t.Fatal("explicit native task selection is unqualified")
		}
		projectKey, taskID = selection.ProjectKey, selection.TaskID
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal("native repository context unavailable")
	}
	repo, err := coordination.InspectRepository(t.Context(), cwd)
	if err != nil {
		t.Fatal("native repository verification unavailable")
	}
	document := bindingDocument{
		Schema:      BindingsSchema,
		Authorities: []Authority{{ID: "explicit-local-mac-qualification", Origin: origin}},
		Bindings: []ProjectBinding{{
			Owner: "native-qualification", RepositoryID: repo.ID, Repository: repo.Repository,
			AuthorityID: "explicit-local-mac-qualification", ProjectKey: projectKey,
			Capabilities: []string{ObserveCapability},
		}},
	}
	bindings, err := ParseBindings(bindingBytes(t, document))
	if err != nil {
		t.Fatal("native qualification binding refused")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	evidence, err := bindings.CheckReadAccess(ctx, "native-qualification", repo, manifest.Credential.Token, nil)
	refusal := os.Getenv("WEBMUX_MAC_NATIVE_EXPECT_REFUSAL") == "1"
	if refusal {
		if !errors.Is(err, ErrReadAccess) || evidence.Schema != "" {
			t.Fatal("revoked own native client did not refuse read access")
		}
	} else if err != nil {
		t.Fatal("native read access failed:", err)
	}
	var task *TaskObservation
	var worker *WorkerObservation
	unownedWorkerRefused := false
	foreignTaskRefused, missingTaskRefused := false, false
	if taskID != "" && !refusal {
		observation, err := bindings.ObserveTask(ctx, "native-qualification", repo, taskID, manifest.Credential.Token, nil)
		if err != nil || observation.Schema != TaskIdentitySchema || observation.SessionLinkState != "unlinked" ||
			observation.MachineIdentityState != "unavailable" || observation.SourceLinkState != "unlinked" {
			t.Fatal("native task identity observation failed:", err)
		}
		task = &observation
		ownerObservation, err := bindings.ObserveTaskWorker(ctx, "native-qualification", repo, taskID, manifest.Credential.Token, nil)
		if observation.OwnerWorkerID == nil {
			if !errors.Is(err, ErrUnmapped) || ownerObservation.Schema != "" {
				t.Fatal("native unowned task acquired an inferred worker")
			}
			unownedWorkerRefused = true
		} else {
			if err != nil || ownerObservation.Schema != WorkerIdentitySchema {
				t.Fatal("native task owner/machine observation failed:", err)
			}
			worker = &ownerObservation
		}
		// The same real task must not cross a different receiving-project binding.
		foreign := document
		foreign.Bindings = append([]ProjectBinding(nil), document.Bindings...)
		foreign.Bindings[0].ProjectKey = "webmux-native-foreign-project-01a2018c"
		if foreign.Bindings[0].ProjectKey == projectKey {
			t.Fatal("native foreign-project control is not independent")
		}
		other, err := ParseBindings(bindingBytes(t, foreign))
		if err != nil {
			t.Fatal("native foreign-project binding refused")
		}
		if got, err := other.ObserveTask(ctx, "native-qualification", repo, taskID, manifest.Credential.Token, nil); !errors.Is(err, ErrUnmapped) || got.Schema != "" {
			t.Fatal("native task crossed its project boundary")
		}
		foreignTaskRefused = true
		if got, err := bindings.ObserveTask(ctx, "native-qualification", repo, "webmux-native-absent-task-01a2018c-20261123", manifest.Credential.Token, nil); !errors.Is(err, ErrUnmapped) || got.Schema != "" {
			t.Fatal("native missing task became an observation")
		}
		missingTaskRefused = true
	}
	// An intentionally nonexistent route has no business operation to execute.
	// MAC middleware must refuse its generic write scope before route dispatch.
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/__webmux_read_scope_probe_no_handler__", nil)
	if err != nil {
		t.Fatal("native scope probe could not be constructed")
	}
	request.Header.Set("Authorization", "Bearer "+manifest.Credential.Token)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("native write-refusal observation unavailable")
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("native client did not refuse generic write scope")
	}
	var project *ProjectObservation
	if taskID != "" && !refusal && os.Getenv("WEBMUX_MAC_NATIVE_PROJECT_VIEW") == "1" {
		observation, err := bindings.ObserveProject(t.Context(), "native-qualification", repo, manifest.Credential.Token, nil, 0, 2)
		if err != nil || observation.Schema != ProjectSchema || observation.ProjectExistence != "unverified" ||
			observation.ConflictVerdict != "unknown" || observation.OrderingVerdict != "unknown" || len(observation.Tasks) > 2 {
			t.Fatal("native bounded project observation failed:", err)
		}
		project = &observation
	}

	runtimeQualified := false
	if project != nil && os.Getenv("WEBMUX_MAC_NATIVE_RUNTIME") == "1" {
		policy := map[string]any{
			"schema": RuntimeSchema, "bindings": document,
			"credentials": []any{map[string]any{"authority_id": "explicit-local-mac-qualification", "client_id": manifest.ClientID,
				"enrollment_file": path, "publisher": map[string]any{"pid": pid, "fingerprint": before}}},
		}
		// A temporary private reference to the original enrollment, never a
		// copied token, installed profile change or runtime backend cutover.
		config := filepath.Join(t.TempDir(), "mac-observation.json")
		encoded, err := json.Marshal(policy)
		if err != nil || os.WriteFile(config, encoded, 0600) != nil {
			t.Fatal("native runtime fixture unavailable")
		}
		runtime := OpenRuntime(config, nil)
		got, err := runtime.ObserveProject(t.Context(), "native-qualification", repo, 0, 2)
		if err != nil || got.Schema != ProjectSchema || got.ProjectKey != project.ProjectKey || got.ConflictVerdict != "unknown" || got.OrderingVerdict != "unknown" {
			t.Fatal("native explicit observer runtime failed:", err)
		}
		runtimeQualified = true
	}
	if publisher() != before {
		t.Fatal("native publisher changed during qualification")
	}
	receipt := struct {
		Schema               string              `json:"schema"`
		ClientID             string              `json:"client_id"`
		ObservedAt           time.Time           `json:"observed_at"`
		CredentialExpiresAt  time.Time           `json:"credential_expires_at"`
		PublisherFingerprint string              `json:"publisher_fingerprint"`
		RepositoryID         string              `json:"repository_id"`
		Read                 ReadEvidence        `json:"read"`
		ExpectedRefusal      bool                `json:"expected_refusal"`
		WriteRefused         bool                `json:"write_refused"`
		Task                 *TaskObservation    `json:"task,omitempty"`
		ForeignTaskRefused   bool                `json:"foreign_task_refused,omitempty"`
		MissingTaskRefused   bool                `json:"missing_task_refused,omitempty"`
		Worker               *WorkerObservation  `json:"worker,omitempty"`
		UnownedWorkerRefused bool                `json:"unowned_worker_refused,omitempty"`
		Project              *ProjectObservation `json:"project,omitempty"`
		RuntimeQualified     bool                `json:"runtime_qualified"`
		Scope                string              `json:"scope"`
	}{"webmux.mac.native-read-qualification.v1", manifest.ClientID, time.Now().UTC(), expires, before,
		repo.ID, evidence, refusal, true, task, foreignTaskRefused, missingTaskRefused, worker, unownedWorkerRefused, project, runtimeQualified,
		"Native read/scope and optional user-supplied fleet-task observations only; test binding is not WebMux project/session enrollment or admission"}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal("native qualification receipt unavailable")
	}
	if output := os.Getenv("WEBMUX_MAC_NATIVE_RECEIPT"); output != "" {
		if os.WriteFile(output, append(encoded, '\n'), 0600) != nil {
			t.Fatal("native receipt could not be retained")
		}
	}
	t.Log("Native publisher remained stable; expected read and generic write-refusal outcomes observed")
}
