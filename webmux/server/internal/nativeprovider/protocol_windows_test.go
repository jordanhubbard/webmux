package nativeprovider

import (
	"bytes"
	"encoding/binary"
	"testing"
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
