package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	plane "github.com/jordanhubbard/webmux/server/internal/coordination"
	"github.com/jordanhubbard/webmux/server/internal/coordinationclient"
)

// Exercise live broker-bound actors, private capabilities, real HTTP and the
// advertised authority together. This fixture never uses the host's live lease.
func TestCoordinationQueueThreeClientHTTPAdmission(t *testing.T) {
	_, _, handler, _, access := coordinationAPI(t)
	server := httptest.NewServer(handler)
	defer server.Close()
	for i := range access {
		access[i].Endpoint = server.URL
		access[i].Socket = ""
	}
	resource := "node:" + access[0].Subject.Node + ":heavy"
	call := func(who int, r plane.Request) (plane.Result, error) {
		data, _ := json.Marshal(r)
		var out bytes.Buffer
		err := coordinationclient.Request(context.Background(), access[who], "POST", "/act", bytes.NewReader(data), &out)
		var result plane.Result
		if err == nil {
			err = json.Unmarshal(out.Bytes(), &result)
		}
		return result, err
	}
	type admission struct {
		who    int
		result plane.Result
		err    error
	}
	results := make(chan admission, 3)
	var group sync.WaitGroup
	for i := range access {
		group.Add(1)
		go func(who int) {
			defer group.Done()
			r, err := call(who, plane.Request{ID: fmt.Sprintf("http-queue-client-%d", who), Action: "enqueue-reservation", Resource: resource})
			results <- admission{who, r, err}
		}(i)
	}
	group.Wait()
	close(results)
	order := []admission{}
	for r := range results {
		if r.err != nil || r.result.Status != "queued" || r.result.Queue == nil {
			t.Fatal(r)
		}
		order = append(order, r)
	}
	sort.Slice(order, func(i, j int) bool { return order[i].result.Queue.Sequence < order[j].result.Queue.Sequence })
	for i, entry := range order {
		barge, err := call(entry.who, plane.Request{ID: fmt.Sprintf("http-legacy-barge-%d", i), Action: "reserve", Resource: resource})
		if err != nil || barge.Status != "busy" {
			t.Fatal(barge, err)
		}
		grant, err := call(entry.who, plane.Request{ID: fmt.Sprintf("http-head-grant-%d", i), Action: "acquire-reservation", Resource: resource, Record: entry.result.Record})
		if err != nil || grant.Status != "reserved" || grant.Fence != uint64(i+1) || grant.Lease == nil || !grant.Lease.Current {
			t.Fatal(grant, err)
		}
		release, err := call(entry.who, plane.Request{ID: fmt.Sprintf("http-head-release-%d", i), Action: "release-reservation", Resource: resource, Record: grant.Record, Fence: grant.Fence})
		if err != nil || release.Status != "released" {
			t.Fatal(release, err)
		}
		t.Logf("actor=%s sequence=%d granted_fence=%d released=true", access[entry.who].Subject.Session, entry.result.Queue.Sequence, grant.Fence)
	}
}
