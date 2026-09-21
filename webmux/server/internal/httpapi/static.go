package httpapi

import (
	"errors"
	"fmt"
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
	root, err := os.OpenRoot(s.webDir)
	if err != nil {
		notFound()
		return
	}
	defer root.Close()
	if name == "" {
		name = "."
	}
	info, err := root.Stat(name)
	if err == nil && info.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			location := *r.URL
			location.Path += "/"
			location.RawPath = ""
			http.Redirect(w, r, location.String(), http.StatusMovedPermanently)
			return
		}
		name = path.Join(name, "index.html")
		info, err = root.Stat(name)
	}
	if errors.Is(err, os.ErrNotExist) {
		name = "index.html"
		info, err = root.Stat(name)
	}
	if err != nil || !info.Mode().IsRegular() {
		notFound()
		return
	}
	// Root.Open confines symlinks at the point of opening, not just during stat.
	file, err := root.Open(name)
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
	w.Header().Set("ETag", fmt.Sprintf(`W/"%x-%x"`, info.Size(), info.ModTime().UnixMilli()))
	for _, directive := range strings.Split(r.Header.Get("Cache-Control"), ",") {
		if strings.EqualFold(strings.TrimSpace(directive), "no-cache") {
			r = r.Clone(r.Context())
			r.Header.Del("If-None-Match")
			r.Header.Del("If-Modified-Since")
			break
		}
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}
