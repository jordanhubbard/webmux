package guacamole

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func TestDecoderFragmentedUnicodeAndSeparators(t *testing.T) {
	raw := "4.test,5.😀;,é.,0.;"
	d := NewDecoder(iotest.OneByteReader(strings.NewReader(raw + Encode("sync", "123"))))
	got, err := d.Read()
	if err != nil || got.Raw != raw || got.Opcode != "test" || !reflect.DeepEqual(got.Args, []string{"😀;,é.", ""}) {
		t.Fatalf("decoded %#v: %v", got, err)
	}
	if Encode(got.Opcode, got.Args...) != raw {
		t.Fatal("encoder counted bytes instead of Unicode characters")
	}
	if next, err := d.Read(); err != nil || next.Opcode != "sync" {
		t.Fatal(next, err)
	}
	if _, err := d.Read(); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func TestDecoderRejectsInvalidAndBoundedInput(t *testing.T) {
	for _, raw := range []string{".a;", "-1.a;", "0.;", "1.a!", "1.\xff;", "9999999.a;", "00000001.a;", "1.a," + strings.Repeat("0.,", 1023) + "0.;", "1048576." + strings.Repeat("a", MaxInstructionBytes) + ";"} {
		if _, err := NewDecoder(strings.NewReader(raw)).Read(); !errors.Is(err, ErrProtocol) {
			t.Errorf("invalid input length %d: %v", len(raw), err)
		}
	}
	for _, raw := range []string{"1", "1.", "2.a", "1.a", "1.a,"} {
		if _, err := NewDecoder(strings.NewReader(raw)).Read(); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("truncated %q: %v", raw, err)
		}
	}
}

func TestHandshakeParametersAndBufferedOutput(t *testing.T) {
	p := Parameters{Hostname: "192.0.2.1", Port: 3389, Username: "user😀", Password: "password", Domain: "domain"}
	names := []string{"hostname", "port", "username", "password", "domain", "width", "height", "dpi", "resize-method", "normalize-clipboard", "enable-drive", "VERSION_1_5_0"}
	want := []string{"192.0.2.1", "3389", "user😀", "password", "domain", "1024", "768", "96", "display-update", "preserve", "false", ""}
	ready := Encode("ready", "connection;id")
	d := NewDecoder(iotest.OneByteReader(strings.NewReader(Encode("args", names...) + ready + Encode("sync", "1"))))
	var output bytes.Buffer
	got, err := Handshake(d, &output, p)
	if err != nil || got.Raw != ready {
		t.Fatal(got, err)
	}
	capabilities := Encode("size", "1024", "768", "96") + Encode("audio") + Encode("video") + Encode("image", "image/png", "image/jpeg")
	if output.String() != Encode("select", "rdp")+capabilities+Encode("connect", want...) {
		t.Fatal("handshake parameter mismatch")
	}
	if next, err := d.Read(); err != nil || next.Opcode != "sync" {
		t.Fatal("lost post-handshake instruction", next, err)
	}
}

func TestHandshakeRejectsSequenceAndRemoteErrors(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want error
	}{
		{Encode("ready", "id"), ErrProtocol},
		{Encode("args") + Encode("args"), ErrProtocol},
		{Encode("error", "remote details", "256"), ErrRemote},
		{strings.Repeat(Encode("unknown"), 256), ErrProtocol},
	} {
		if _, err := Handshake(NewDecoder(strings.NewReader(tc.raw)), io.Discard, Parameters{}); !errors.Is(err, tc.want) {
			t.Fatal(err, tc.want)
		}
	}
}
