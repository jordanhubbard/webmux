package session

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

// Preserve the previous reverse-scan implementation as an independent oracle.
func referenceScrollback(value string) string {
	units, start := 0, len(value)
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(value[:start])
		width := 1
		if r > 0xffff {
			width = 2
		}
		if units+width > 64*1024 {
			break
		}
		units += width
		start -= size
	}
	if start == 0 {
		return value
	}
	value = value[start:]
	if newline := strings.IndexByte(value, '\n'); newline >= 0 && len(utf16.Encode([]rune(value[:newline]))) < 4096 {
		value = value[newline+1:]
	}
	return value
}

func TestIncrementalScrollbackMatchesReference(t *testing.T) {
	random := rand.New(rand.NewPCG(1, 2))
	pieces := []string{"abc", "\n", "😀", "日本語", "\x00", "\r", strings.Repeat("x", 4096), strings.Repeat("λ😀\n", 30000)}
	var got, want string
	units := 0
	for i := 0; i < 1000; i++ {
		data := pieces[random.IntN(len(pieces))]
		want = referenceScrollback(want + data)
		got, units = appendScrollback(got, units, data)
		if got != want || units != len(utf16.Encode([]rune(got))) || units > 65536 {
			t.Fatalf("iteration %d: bytes %d/%d, units %d", i, len(got), len(want), units)
		}
	}
}

func BenchmarkScrollbackAppend(b *testing.B) {
	for _, entry := range []struct {
		name   string
		append func(string, int, string) (string, int)
	}{
		{"reference", func(previous string, units int, data string) (string, int) {
			return referenceScrollback(previous + data), 0
		}},
		{"incremental", appendScrollback},
	} {
		b.Run(entry.name, func(b *testing.B) {
			value, units := strings.Repeat("x", 65536), 65536
			chunk := strings.Repeat("x", 4096)
			b.ReportAllocs()
			b.SetBytes(int64(len(chunk)))
			for b.Loop() {
				value, units = entry.append(value, units, chunk)
			}
		})
	}
}
