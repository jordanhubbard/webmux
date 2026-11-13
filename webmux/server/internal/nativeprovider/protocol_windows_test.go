package nativeprovider

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProviderHistoryRetainsBoundedOrderedTail(t *testing.T) {
	var h history
	all := bytes.Repeat([]byte("abcdefg"), historyLimit/3)
	for n := 0; n < len(all); {
		end := min(n+7919, len(all))
		h.add(all[n:end])
		n = end
	}
	if got := h.snapshot(); !bytes.Equal(got, all[len(all)-historyLimit:]) {
		t.Fatal("provider history lost order or exceeded its bound")
	}
	h.add(bytes.Repeat([]byte("x"), historyLimit+77))
	if got := h.snapshot(); len(got) != historyLimit || !bytes.Equal(got, bytes.Repeat([]byte("x"), historyLimit)) {
		t.Fatal("large write retained an unbounded or incorrect history")
	}
}

func TestProviderProbeCancellationWakesEstablishedHandshake(t *testing.T) {
	dir, err := os.MkdirTemp("", "wmp-cancel-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := secureCreatedDirectory(dir); err != nil {
		t.Fatal(err)
	}
	endpoint := filepath.Join(dir, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		var request frame
		if readFrame(c, &request) != nil {
			return
		}
		close(entered)
		<-release
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := Probe(ctx, Identity{Session: "fixture", Instance: "fixture", OwnerPID: os.Getpid(), Endpoint: endpoint}, "fixture")
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not enter handshake")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wrong cancellation outcome: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled handshake waited for its original deadline")
	}
}

func TestProviderFramesRejectOversizeAndMultipleValues(t *testing.T) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], maxFrame+1)
	var value frame
	if err := readFrame(bytes.NewReader(header[:]), &value); err == nil {
		t.Fatal("accepted oversized frame")
	}
	data := []byte(`{"kind":"probe"}{"kind":"stop"}`)
	binary.BigEndian.PutUint32(header[:], uint32(len(data)))
	if err := readFrame(bytes.NewReader(append(header[:], data...)), &value); err == nil {
		t.Fatal("accepted multiple control messages in one frame")
	}
}
