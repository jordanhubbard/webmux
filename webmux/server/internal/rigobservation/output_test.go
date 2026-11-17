package rigobservation

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRigOutputCopyBound(t *testing.T) {
	const limit = 131072
	for _, size := range []int{0, limit - 1, limit, limit + 1, limit + 65536} {
		for _, limited := range []bool{false, true} {
			t.Run(fmt.Sprintf("bytes-%d-reader-only-%t", size, limited), func(t *testing.T) {
				var out boundedOutput
				var reader io.Reader = strings.NewReader(strings.Repeat("x", size))
				if limited {
					reader = io.LimitReader(reader, int64(size))
				} // No WriterTo: exercise ReaderFrom dispatch too.
				_, err := io.Copy(&out, reader)
				if len(out.Bytes()) > limit {
					t.Fatalf("retained %d bytes past %d limit", len(out.Bytes()), limit)
				}
				if size <= limit {
					if err != nil || len(out.Bytes()) != size {
						t.Fatal("valid response rejected", err, len(out.Bytes()))
					}
				} else if err == nil {
					t.Fatal("oversized copy accepted")
				}
			})
		}
	}
}

func TestRigOutputChildCaptureBound(t *testing.T) {
	const limit = 131072
	if os.Getenv("WEBMUX_TEST_RIG_OUTPUT_HELPER") == "1" {
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", limit+65536)))
		os.Exit(0)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestRigOutputChildCaptureBound$")
	cmd.Env = append(os.Environ(), "WEBMUX_TEST_RIG_OUTPUT_HELPER=1")
	var out boundedOutput
	cmd.Stdout = &out
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("oversized producer did not finish within deadline", ctx.Err())
	}
	if len(out.Bytes()) > limit {
		t.Fatalf("child retained %d bytes past %d limit", len(out.Bytes()), limit)
	}
	if err == nil {
		t.Fatal("oversized child response accepted")
	}
}
