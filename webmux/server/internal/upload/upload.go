// Package upload stores authenticated uploads used by the SSH key catalog.
package upload

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/storage"
)

const MaxFileSize int64 = 10 * 1024 * 1024
const QuotaBytes int64 = 500 * 1024 * 1024
const MaxAge = 30 * 24 * time.Hour

var ErrTooLarge = errors.New("File too large (max 10 MB)")
var ErrQuota = errors.New("Upload quota exceeded (524288000 bytes total)")
var safeName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type Result struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}
type PurgeResult struct {
	Deleted    int
	FreedBytes int64
}
type Service struct {
	store     *storage.Store
	directory string
	gate      chan struct{}
	quota     int64
	maxFile   int64
	now       func() time.Time
	closed    bool
}

func New(store *storage.Store) *Service {
	return &Service{store: store, directory: filepath.Join(store.Home, "uploads"), gate: make(chan struct{}, 1), quota: QuotaBytes, maxFile: MaxFileSize, now: time.Now}
}

func (s *Service) lock(ctx context.Context) error {
	select {
	case s.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			s.unlock()
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Service) unlock() { <-s.gate }

func filename(prefix, raw string) string {
	base := filepath.Base(raw)
	if raw != "" && safeName.MatchString(base) && len(base) <= 240 {
		return prefix + "-" + base
	}
	ext := filepath.Ext(raw)
	if len(ext) > 64 || !safeName.MatchString(ext) {
		ext = ""
	}
	if ext == "" {
		ext = ".bin"
	}
	return prefix + ext
}

func regularFiles(root *os.Root) ([]os.FileInfo, error) {
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	files := []os.FileInfo{}
	for _, entry := range entries {
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode().IsRegular() {
			files = append(files, info)
		}
	}
	return files, nil
}

func usage(root *os.Root) (int64, error) {
	files, err := regularFiles(root)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, info := range files {
		total += info.Size()
	}
	return total, nil
}

// Store serializes writers and cleanup so quota accounting includes every
// committed upload. Cancellation while waiting for a writer does not consume a slot.
func (s *Service) Store(ctx context.Context, rawName string, body io.Reader) (Result, error) {
	if err := s.lock(ctx); err != nil {
		return Result{}, err
	}
	defer s.unlock()
	if s.closed {
		return Result{}, os.ErrClosed
	}
	if err := os.MkdirAll(s.directory, 0700); err != nil {
		return Result{}, err
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	used, err := usage(root)
	if err != nil {
		return Result{}, err
	}
	if used+s.maxFile > s.quota {
		if _, err := s.purge(root); err != nil {
			return Result{}, err
		}
		used, err = usage(root)
		if err != nil {
			return Result{}, err
		}
	}
	if used >= s.quota {
		return Result{}, ErrQuota
	}
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Result{}, err
	}
	prefix := hex.EncodeToString(random[:])
	name := filename(prefix, rawName)
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Result{}, err
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = root.Remove(name)
		}
	}()
	limit := min(s.maxFile, s.quota-used)
	size, err := io.Copy(file, io.LimitReader(body, limit+1))
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if size > s.maxFile {
		return Result{}, ErrTooLarge
	}
	if size > s.quota-used {
		return Result{}, ErrQuota
	}
	if err := file.Sync(); err != nil {
		return Result{}, err
	}
	if err := file.Close(); err != nil {
		return Result{}, err
	}
	// Only publish the path after the entire file is synced. Exclusive creation
	// prevents a nonce collision from overwriting an existing key.
	committed = true
	return Result{Path: filepath.Join(s.directory, name), Name: name, Size: size}, nil
}

func canonical(name string) string {
	if absolute, err := filepath.Abs(name); err == nil {
		name = absolute
	}
	if resolved, err := filepath.EvalSymlinks(name); err == nil {
		name = resolved
	}
	if runtime.GOOS == "windows" {
		name = strings.ToLower(name)
	}
	return filepath.Clean(name)
}

func (s *Service) purge(root *os.Root) (PurgeResult, error) {
	var result PurgeResult
	type keyConfig struct {
		Keys []struct {
			Path string `yaml:"private_key_path"`
		} `yaml:"keys"`
	}
	// An unreadable catalog must never make all existing key files disposable.
	err := storage.InspectConfig(s.store, "keys.yaml", func(config *keyConfig) error {
		referenced := map[string]bool{}
		for _, key := range config.Keys {
			if key.Path != "" {
				referenced[canonical(key.Path)] = true
			}
		}
		files, err := regularFiles(root)
		if err != nil {
			return err
		}
		for _, info := range files {
			if referenced[canonical(filepath.Join(s.directory, info.Name()))] || !s.now().After(info.ModTime().Add(MaxAge)) {
				continue
			}
			if err := root.Remove(info.Name()); err != nil {
				continue
			}
			result.Deleted++
			result.FreedBytes += info.Size()
		}
		return nil
	})
	return result, err
}

func (s *Service) Purge(ctx context.Context) (PurgeResult, error) {
	if err := s.lock(ctx); err != nil {
		return PurgeResult{}, err
	}
	defer s.unlock()
	if s.closed {
		return PurgeResult{}, os.ErrClosed
	}
	root, err := os.OpenRoot(s.directory)
	if errors.Is(err, os.ErrNotExist) {
		return PurgeResult{}, nil
	}
	if err != nil {
		return PurgeResult{}, err
	}
	defer root.Close()
	return s.purge(root)
}

// Close follows HTTP shutdown, which cancels active request-body reads. Taking
// the writer slot waits for their file cleanup before the process exits.
func (s *Service) Close() error {
	s.gate <- struct{}{}
	defer s.unlock()
	s.closed = true
	return nil
}
