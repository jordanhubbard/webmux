package coordinationclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/coordination"
)

// waitReservation is a bounded client-side wait, never a second queue or a
// lease acquisition. MAC can run it as an executor subprocess without parking
// a model turn. Every observation reauthenticates through the ordinary API.
func waitReservation(args []string, access Access, out io.Writer) error {
	flags := flag.NewFlagSet("coordinate wait", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	resource := flags.String("resource", "", "authority resource")
	record := flags.String("record-id", "", "queue ticket")
	id := flags.String("request-id", "", "stable prefix for this wait attempt")
	seconds := flags.Int("timeout-seconds", 20, "bounded wait, 1-30 seconds")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *resource == "" || *record == "" || len(*id) < 8 || len(*id) > 100 || *seconds < 1 || *seconds > 30 {
		return errors.New("usage: coordinate wait --resource ID --record-id TICKET --request-id ID --timeout-seconds 1..30")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*seconds)*time.Second)
	defer cancel()
	var last []byte
	timedOut := func() error {
		if _, err := out.Write(last); err != nil {
			return err
		}
		return errors.New("queue wait deadline reached; ticket remains queued until cancelled or its authority deadline")
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for attempt := 0; ; attempt++ {
		if ctx.Err() != nil {
			return timedOut()
		}
		request := coordination.Request{ID: fmt.Sprintf("%s-wait-%d", *id, attempt), Action: "inspect-reservation", Resource: *resource, Record: *record}
		body, _ := json.Marshal(request)
		var response bytes.Buffer
		if err := Request(ctx, access, "POST", "/act", bytes.NewReader(body), &response); err != nil {
			if ctx.Err() != nil {
				return timedOut()
			}
			return err
		}
		last = append(last[:0], response.Bytes()...)
		var result coordination.Result
		if err := json.Unmarshal(response.Bytes(), &result); err != nil {
			return err
		}
		if result.Status != "queued" || result.Queue == nil || result.Queue.Ready {
			if _, err := out.Write(response.Bytes()); err != nil {
				return err
			}
			if result.Status == "queued" && result.Queue != nil && result.Queue.Ready {
				return nil
			}
			return errors.New("queue wait ended: " + result.Status)
		}
		select {
		case <-ctx.Done():
			return timedOut()
		case <-tick.C:
		}
	}
}
