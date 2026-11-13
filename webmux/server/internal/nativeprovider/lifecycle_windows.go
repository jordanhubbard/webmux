package nativeprovider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func readRecord(root, session, key string) (Identity, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return Identity{}, err
	}
	defer r.Close()
	stat, err := r.Lstat("identity.json")
	if err != nil {
		return Identity{}, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > 8192 {
		return Identity{}, errors.New("invalid native provider record")
	}
	f, err := r.Open("identity.json")
	if err != nil {
		return Identity{}, err
	}
	defer f.Close()
	var saved record
	d := json.NewDecoder(io.LimitReader(f, 8193))
	d.DisallowUnknownFields()
	if err := d.Decode(&saved); err != nil {
		return Identity{}, errors.New("invalid native provider record")
	}
	if saved.Identity.Session != session || !equal(saved.KeyDigest, secretDigest(key)) {
		return Identity{}, errors.New("native provider record authentication failed")
	}
	i := saved.Identity
	if len(i.Instance) != 32 || !filepath.IsAbs(i.Endpoint) || i.OwnerPID <= 0 || len(i.BinarySHA256) != 64 {
		return Identity{}, errors.New("invalid native provider identity record")
	}
	return i, nil
}

func writeRecord(root, key string, i Identity) error {
	r, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer r.Close()
	if stat, err := r.Lstat("identity.json"); err == nil && !stat.Mode().IsRegular() {
		return errors.New("native provider record is not a regular file")
	}
	name := "identity-" + i.Instance + ".tmp"
	f, err := r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer r.Remove(name)
	data, err := json.Marshal(record{Identity: i, KeyDigest: secretDigest(key)})
	if err == nil {
		_, err = f.Write(data)
	}
	err = errors.Join(err, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	return r.Rename(name, "identity.json")
}

// Cache an immutable image: a surviving provider must not keep the installer's
// controller executable mapped. All sessions share this account-owned cache.
func pinExecutable(c *Config) (string, error) {
	f, err := os.Open(c.Executable)
	if err != nil {
		return "", err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128<<20 {
		f.Close()
		return "", errors.New("invalid native provider helper image")
	}
	data, err := io.ReadAll(io.LimitReader(f, (128<<20)+1))
	f.Close()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	sum := hex.EncodeToString(digest[:])
	dir := filepath.Join(filepath.Dir(c.Root), ".helpers")
	if err := privateDirectory(dir); err != nil {
		return "", err
	}
	name := filepath.Join(dir, sum+".exe")
	verify := func() bool {
		s, err := os.Lstat(name)
		if err != nil || !s.Mode().IsRegular() || s.Size() != int64(len(data)) {
			return false
		}
		existing, err := os.ReadFile(name)
		return err == nil && bytes.Equal(existing, data)
	}
	if _, err := os.Lstat(name); err == nil {
		if !verify() {
			return "", errors.New("native provider helper cache integrity mismatch")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else {
		temp, err := os.CreateTemp(dir, ".helper-*")
		if err != nil {
			return "", err
		}
		defer os.Remove(temp.Name())
		_, err = temp.Write(data)
		err = errors.Join(err, temp.Sync(), temp.Close())
		if err != nil {
			return "", err
		}
		if err := os.Rename(temp.Name(), name); err != nil && !verify() {
			return "", err
		}
	}
	if strings.EqualFold(filepath.Clean(c.Launch.Program), filepath.Clean(c.Executable)) {
		c.Launch.Program = name
	}
	for n, value := range c.Launch.Environment {
		key, value, ok := strings.Cut(value, "=")
		if ok && strings.EqualFold(key, "WEBMUX_RECOVERY_HELPER") && strings.EqualFold(value, c.Executable) {
			c.Launch.Environment[n] = key + "=" + name
		}
	}
	c.Executable = name
	return sum, nil
}

func detached(command *exec.Cmd) {
	// Refuse a restrictive outer job rather than pretending that an attached
	// child can survive it. The second hop also outlives parent-tree cleanup.
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_BREAKAWAY_FROM_JOB}
}

// Ensure never decides that a timeout means the job died. Only the session's
// lifetime mutex authorizes a new owner. The record can be stale or missing
// without allowing another job while the old owner is still alive.
func Ensure(ctx context.Context, c Config) (Identity, bool, error) {
	if err := ctx.Err(); err != nil {
		return Identity{}, false, err
	}
	c.Launch.Environment = slices.Clone(c.Launch.Environment)
	if err := validate(c); err != nil {
		return Identity{}, false, err
	}
	if err := prepareRoot(c.Root); err != nil {
		return Identity{}, false, err
	}
	var previous Identity
	if i, err := readRecord(c.Root, c.Session, c.Key); err == nil {
		previous = i
		if result, err := Probe(ctx, i, c.Key); err == nil {
			return result, true, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Identity{}, false, err
	}
	if _, err := pinExecutable(&c); err != nil {
		return Identity{}, false, err
	}
	var payload bytes.Buffer
	if err := writeFrame(&payload, c); err != nil {
		return Identity{}, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, c.Executable, append(append([]string{}, c.DriverArgs...), "--native-provider-bootstrap")...)
	command.Stdin = &payload
	detached(command)
	out, startErr := command.Output()
	if errors.Is(startErr, windows.ERROR_ACCESS_DENIED) {
		return Identity{}, false, errors.New("the outer Windows job does not permit an independent native provider")
	}
	var reply frame
	if startErr == nil && readFrame(bytes.NewReader(out), &reply) == nil && reply.Kind == "ready" && reply.Error == "" && reply.Identity != nil {
		if i, err := Probe(ctx, *reply.Identity, c.Key); err == nil {
			return i, i.Instance == previous.Instance, nil
		}
	}
	// A lost bootstrap reply is not a reason to start another owner. Resolve
	// the published record, including concurrent creation, within this budget.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if i, err := readRecord(c.Root, c.Session, c.Key); err == nil {
			if result, err := Probe(ctx, i, c.Key); err == nil {
				return result, i.Instance == previous.Instance, nil
			}
		}
		select {
		case <-ctx.Done():
			return Identity{}, false, errors.New("native provider startup could not be confirmed; no replacement job was authorized")
		case <-ticker.C:
		}
	}
}

// Bootstrap exits after handing its private startup pipe to the independent
// owner. No credentials are placed in argv, a durable record or a transcript.
func Bootstrap(ctx context.Context, input io.Reader, output io.Writer) error {
	var c Config
	if err := readFrame(input, &c); err != nil {
		return err
	}
	if err := validate(c); err != nil {
		return err
	}
	command := exec.Command(c.Executable, append(append([]string{}, c.DriverArgs...), "--native-provider-owner")...)
	var payload bytes.Buffer
	if err := writeFrame(&payload, c); err != nil {
		return err
	}
	command.Stdin = &payload
	detached(command)
	pipe, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	defer pipe.Close()
	if err := command.Start(); err != nil {
		return err
	}
	_ = command.Process.Release()
	result := make(chan frame, 1)
	go func() {
		var reply frame
		if err := readFrame(pipe, &reply); err != nil {
			reply = frame{Kind: "ready", Error: "native provider startup reply unavailable"}
		}
		result <- reply
	}()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case reply := <-result:
		return writeFrame(output, reply)
	case <-ctx.Done():
		return writeFrame(output, frame{Kind: "ready", Error: "native provider startup is uncertain"})
	}
}
