package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/upload"
)

func TestUploadAuthenticationBinaryDataAndLimit(t *testing.T) {
	_, local := fixture(t, "local")
	requireStatus(t, request(local, "POST", "/api/upload", "", ""), 401)
	s, trusted := fixture(t, "none")
	requireStatus(t, request(trusted, "POST", "/api/upload", `{}`, ""), 400)
	for _, size := range []int{0, 256, int(upload.MaxFileSize), int(upload.MaxFileSize) + 1} {
		data := bytes.Repeat([]byte{255}, size)
		r := httptest.NewRequest("POST", "/api/upload/", bytes.NewReader(data))
		r.Header.Set("Content-Type", "application/octet-stream")
		r.Header.Set("X-Filename", "key.pem")
		response := httptest.NewRecorder()
		trusted.ServeHTTP(response, r)
		if int64(size) > upload.MaxFileSize {
			requireStatus(t, response, 413)
			continue
		}
		requireStatus(t, response, 201)
		var result upload.Result
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		stored, err := os.ReadFile(result.Path)
		if err != nil || !bytes.Equal(stored, data) || result.Size != int64(size) {
			t.Fatal("uploaded bytes differ", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.MaintainUploads(ctx) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("upload maintenance did not stop")
	}
}
