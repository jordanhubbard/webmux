package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/upload"
)

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request, _ string) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/octet-stream") {
		writeError(w, 400, "Content-Type must be application/octet-stream")
		return
	}
	value, err := s.uploads.Store(r.Context(), r.Header.Get("X-Filename"), r.Body)
	if err != nil {
		switch {
		case errors.Is(err, upload.ErrTooLarge):
			writeError(w, 413, err.Error())
		case errors.Is(err, upload.ErrQuota):
			writeError(w, 507, err.Error())
		default:
			s.logger.Error("upload failed", "error", err)
			writeError(w, 500, "Upload failed")
		}
		return
	}
	writeJSON(w, 201, value)
}

// MaintainUploads runs once at startup and daily until server shutdown.
func (s *Server) MaintainUploads(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		result, err := s.uploads.Purge(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("upload purge failed", "error", err)
		}
		if result.Deleted > 0 {
			s.logger.Info("upload purge", "deleted", result.Deleted, "freed_bytes", result.FreedBytes)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
