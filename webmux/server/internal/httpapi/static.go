package httpapi

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
)

// serveUI serves the existing production build. API routes take precedence;
// unresolved GET routes fall back to index.html for client-side navigation.
func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	notFound := func() { http.NotFound(w, r) }
	name := strings.TrimPrefix(r.URL.Path, "/")
	if strings.ContainsAny(name, "\\:\x00") {
		notFound()
		return
	}
	for _, part := range strings.Split(name, "/") {
		if strings.HasPrefix(part, ".") {
			notFound()
			return
		}
	}
	files := s.webFS
	if s.webDir != "" {
		root, err := os.OpenRoot(s.webDir)
		if err != nil {
			notFound()
			return
		}
		defer root.Close()
		files = root.FS()
	}
	if files == nil {
		notFound()
		return
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		name = "."
	}
	info, err := fs.Stat(files, name)
	if err == nil && info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			location := *r.URL
			location.Path += "/"
			location.RawPath = ""
			http.Redirect(w, r, location.String(), http.StatusMovedPermanently)
			return
		}
		name = path.Join(name, "index.html")
		info, err = fs.Stat(files, name)
	}
	if errors.Is(err, os.ErrNotExist) {
		name = "index.html"
		info, err = fs.Stat(files, name)
	}
	if err != nil || !info.Mode().IsRegular() {
		notFound()
		return
	}
	// Root.Open confines symlinks at the point of opening, not just during stat.
	file, err := files.Open(name)
	if err != nil {
		notFound()
		return
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		notFound()
		return
	}
	contentType := mime.TypeByExtension(path.Ext(name))
	switch strings.ToLower(path.Ext(name)) {
	case ".js", ".mjs":
		contentType = "application/javascript; charset=UTF-8"
	case ".html":
		contentType = "text/html; charset=UTF-8"
	case ".css":
		contentType = "text/css; charset=UTF-8"
	case ".json", ".map":
		contentType = "application/json; charset=UTF-8"
	case ".svg":
		contentType = "image/svg+xml"
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=0")
	var content io.ReadSeeker
	if info.ModTime().IsZero() {
		// Embedded files have no modification time. Hash their bytes so an upgrade
		// invalidates cached files even when their lengths happen to match.
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Unable to read asset", 500)
			return
		}
		content = bytes.NewReader(data)
		w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(data)))
	} else {
		var ok bool
		content, ok = file.(io.ReadSeeker)
		if !ok {
			http.Error(w, "Unable to seek asset", 500)
			return
		}
		w.Header().Set("ETag", fmt.Sprintf(`W/"%x-%x"`, info.Size(), info.ModTime().UnixMilli()))
	}
	for _, directive := range strings.Split(r.Header.Get("Cache-Control"), ",") {
		if strings.EqualFold(strings.TrimSpace(directive), "no-cache") {
			r = r.Clone(r.Context())
			r.Header.Del("If-None-Match")
			r.Header.Del("If-Modified-Since")
			break
		}
	}
	http.ServeContent(w, r, name, info.ModTime(), content)
}
