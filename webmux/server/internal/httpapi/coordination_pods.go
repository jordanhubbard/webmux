package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/openrigcoord"
)

var rigPodName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

func (s *Server) coordinationRigPods(w http.ResponseWriter, r *http.Request, owner string) {
	if !s.openRigAccess(w, owner) {
		return
	}
	var input struct {
		ProjectID   string `json:"projectId"`
		ProjectName string `json:"projectName"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err != nil || !json.Valid(body) || decoder.Decode(&input) != nil || !enrollmentID.MatchString(input.ProjectID) || input.ProjectName == "" || len(input.ProjectName) > 256 || isRemoteRigTarget(input.ProjectID) {
		writeError(w, 400, "Choose a canonical local Rig project.")
		return
	}
	provider := s.coordinationProvider(owner, "rig")
	if provider == nil || !provider.Descriptor().Available {
		writeError(w, 409, "Rig is unavailable or disabled.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	payload, _ := json.Marshal(input)
	output, err := openrigcoord.Run(ctx, s.store.Home, owner, "project-pods", string(payload))
	if err != nil {
		writeError(w, 409, "Rig could not verify this project's current pods. Refresh and choose again.")
		return
	}
	var result struct {
		ProjectID   string   `json:"projectId"`
		ProjectName string   `json:"projectName"`
		Pods        []string `json:"pods"`
		Source      string   `json:"source"`
	}
	if json.Unmarshal(output, &result) != nil || result.ProjectID != input.ProjectID || result.ProjectName != input.ProjectName || (result.Source != "native-spec" && result.Source != "legacy-seats") || result.Pods == nil || len(result.Pods) > 4096 {
		writeError(w, 502, "Rig returned an unverified project namespace.")
		return
	}
	seen := map[string]bool{}
	for _, pod := range result.Pods {
		if !rigPodName.MatchString(pod) || seen[pod] {
			writeError(w, 502, "Rig returned an ambiguous project namespace.")
			return
		}
		seen[pod] = true
	}
	writeJSON(w, http.StatusOK, result)
}
