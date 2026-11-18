//go:build linux || darwin

package nativeobserve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReferenceReplacesLinkWithoutFollowingIt(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(foreign, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join(root, ReferenceFile)
	if err := os.Symlink(foreign, reference); err != nil {
		t.Fatal(err)
	}
	if err := WriteReference(root); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(foreign)
	if err != nil || string(bytes) != "retained" {
		t.Fatalf("foreign file changed: %v", err)
	}
	stat, err := os.Lstat(reference)
	if err != nil || !stat.Mode().IsRegular() || stat.Mode().Perm() != 0600 {
		t.Fatalf("unsafe reference: %v %v", stat, err)
	}
	bytes, err = os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	var ref Reference
	if err = json.Unmarshal(bytes, &ref); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(ref.Executable)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(source)
	if ref.Contract != Contract || ref.ExecutableSHA256 != hex.EncodeToString(digest[:]) {
		t.Fatal("helper executable provenance changed")
	}
	leftovers, err := filepath.Glob(filepath.Join(root, ".native-observer-*.tmp"))
	if err != nil || len(leftovers) != 0 {
		t.Fatalf("temporary reference retained: %v %v", leftovers, err)
	}
}

func TestSnapshotIsPrivateImmutableAndNeverRepairedOverCorruption(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WriteReference(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ReferenceFile))
	if err != nil {
		t.Fatal(err)
	}
	var ref Reference
	if err = json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(ref.Executable)
	if err != nil || info.Mode().Perm() != 0500 {
		t.Fatalf("snapshot permissions: %v %v", info, err)
	}
	if err = WriteReference(root); err != nil {
		t.Fatal(err)
	}
	again, err := os.Stat(ref.Executable)
	if err != nil || !info.ModTime().Equal(again.ModTime()) {
		t.Fatal("immutable helper rewritten")
	}
	if err = os.Chmod(ref.Executable, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(ref.Executable, []byte("corrupted"), 0500); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(ref.Executable, 0500); err != nil {
		t.Fatal(err)
	}
	if err = WriteReference(root); err == nil {
		t.Fatal("corrupted helper was silently repaired")
	}
	data, err = os.ReadFile(ref.Executable)
	if err != nil || string(data) != "corrupted" {
		t.Fatal("corruption evidence overwritten")
	}
}

func TestReferenceRefusesAnExistingSharedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteReference(root); err == nil {
		t.Fatal("shared controller namespace accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "controller-observers")); !os.IsNotExist(err) {
		t.Fatal("snapshot created in untrusted namespace")
	}
}
