package httpapi

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/config"
	"github.com/jordanhubbard/webmux/server/internal/storage"
)

func (s *Server) loadSettings() (config.Document, error) {
	var document config.Document
	if err := s.store.ReadConfig("app.yaml", &document); err != nil {
		return config.Document{}, err
	}
	return config.Normalize(document, true)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, _ string) {
	document, err := s.loadSettings()
	if err != nil {
		s.catalogError(w, err, "Failed to load config")
		return
	}
	if command := os.Getenv("WEBMUX_EXEC_COMMAND"); command != "" {
		document.App["exec_command"] = command
	}
	writeJSON(w, 200, config.WithFontURLs(document))
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request, actor string) {
	authConfig, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if authConfig.Auth.Mode != "none" {
		if err := checkAdmin(&authConfig, actor); err != nil {
			s.changeError(w, err)
			return
		}
	}
	var body map[string]any
	if !decodeJSON(w, r, &body) {
		return
	}
	updates, ok := body["app"].(map[string]any)
	if !ok {
		writeError(w, 400, "Request body must contain an app object")
		return
	}
	var response config.Document
	err = storage.UpdateConfig(s.store, "app.yaml", func(current *config.Document) error {
		persisted, err := config.Update(*current, updates)
		if err != nil {
			return err
		}
		response, err = config.Normalize(persisted, true)
		if err != nil {
			return err
		}
		*current = persisted
		return nil
	})
	if err != nil {
		var validation *config.ValidationError
		if errors.As(err, &validation) {
			writeError(w, 400, validation.Error())
			return
		}
		s.catalogError(w, err, "Failed to save config")
		return
	}
	writeJSON(w, 200, config.WithFontURLs(response))
}

func (s *Server) getLayout(w http.ResponseWriter, r *http.Request, _ string) {
	var document map[string]any
	if err := s.store.ReadConfig("layout.yaml", &document); err != nil {
		s.catalogError(w, err, "Failed to load layout")
		return
	}
	writeJSON(w, 200, document)
}

func (s *Server) updateLayout(w http.ResponseWriter, r *http.Request, _ string) {
	var document map[string]any
	if !decodeJSON(w, r, &document) {
		return
	}
	if _, ok := document["layout"].(map[string]any); !ok {
		writeError(w, 400, "Request body must contain a layout object")
		return
	}
	if err := s.store.WriteConfig("layout.yaml", document); err != nil {
		s.catalogError(w, err, "Failed to save layout")
		return
	}
	writeJSON(w, 200, document)
}

func (s *Server) getFont(w http.ResponseWriter, r *http.Request, _ string) {
	notFound := func() { writeError(w, 404, "Font not found") }
	index, err := config.Number(r.PathValue("index"))
	if err != nil || math.IsNaN(index) || math.IsInf(index, 0) || index < 0 || math.Trunc(index) != index {
		notFound()
		return
	}
	document, err := s.loadSettings()
	if err != nil {
		notFound()
		return
	}
	faces, ok := document.App["font_faces"].([]config.FontFace)
	if !ok || index >= float64(len(faces)) {
		notFound()
		return
	}
	appFile, err := filepath.EvalSymlinks(s.store.ConfigPath("app.yaml"))
	if err != nil {
		notFound()
		return
	}
	directory := filepath.Dir(appFile)
	filename, err := filepath.EvalSymlinks(filepath.Join(directory, faces[int(index)].Source))
	if err != nil {
		notFound()
		return
	}
	relative, err := filepath.Rel(directory, filename)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		notFound()
		return
	}
	contentType := config.FontContentType(filename)
	if contentType == "" {
		notFound()
		return
	}
	// Root.Open also enforces containment while opening the file, closing the
	// symlink replacement race between realpath validation and the actual read.
	root, err := os.OpenRoot(directory)
	if err != nil {
		notFound()
		return
	}
	defer root.Close()
	info, err := root.Stat(relative)
	if err != nil || !info.Mode().IsRegular() {
		notFound()
		return
	}
	file, err := root.Open(relative)
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
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	// Match Express's weak stat ETag so browser font caches survive a switch
	// between backends as well as ordinary conditional requests.
	w.Header().Set("ETag", fmt.Sprintf(`W/"%x-%x"`, info.Size(), info.ModTime().UnixMilli()))
	for _, directive := range strings.Split(r.Header.Get("Cache-Control"), ",") {
		if strings.EqualFold(strings.TrimSpace(directive), "no-cache") {
			r = r.Clone(r.Context())
			r.Header.Del("If-None-Match")
			r.Header.Del("If-Modified-Since")
			break
		}
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
