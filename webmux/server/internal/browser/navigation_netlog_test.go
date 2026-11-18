package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Test-only projection. Never forward raw NetLog dictionaries, parameters,
// headers, URLs, DNS names, addresses, command lines or profile information.
type firstNavigationNetLog struct {
	Bytes           int                       `json:"bytes"`
	Scanned         int                       `json:"scanned"`
	MatchedRequests int                       `json:"matchedRequests"`
	Sources         int                       `json:"sources"`
	SourcesLimited  bool                      `json:"sourcesLimited,omitempty"`
	Dropped         int                       `json:"dropped"`
	Events          []firstNavigationNetEvent `json:"events"`
}

type firstNavigationNetEvent struct {
	Source     int    `json:"source"`
	SourceType int    `json:"sourceType"`
	Dependency int    `json:"dependency,omitempty"`
	Type       int    `json:"type"`
	Name       string `json:"name"`
	Phase      int    `json:"phase"`
	NetError   *int   `json:"netError,omitempty"`
	Time       int64  `json:"relativeMs"`
}

func readFirstNavigationNetLog(path, requested string) (*firstNavigationNetLog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const maxBytes = 8 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return nil, fmt.Errorf("network diagnostic input exceeds its bound or cannot be read")
	}
	type source struct {
		ID   int `json:"id"`
		Type int `json:"type"`
	}
	var raw struct {
		Constants struct {
			EventTypes map[string]int `json:"logEventTypes"`
		} `json:"constants"`
		Events []struct {
			Source source `json:"source"`
			Type   int    `json:"type"`
			Phase  int    `json:"phase"`
			Time   string `json:"time"`
			Params struct {
				URL        string  `json:"url"`
				NetError   *int    `json:"net_error"`
				Dependency *source `json:"source_dependency"`
			} `json:"params"`
		} `json:"events"`
	}
	if err = json.Unmarshal(data, &raw); err != nil || len(raw.Events) > 32768 {
		return nil, fmt.Errorf("network diagnostic input is invalid or has too many events")
	}
	result := &firstNavigationNetLog{Bytes: len(data), Scanned: len(raw.Events), Events: []firstNavigationNetEvent{}}
	selected := map[int]int{}
	for _, event := range raw.Events {
		if requested != "" && event.Params.URL == requested && event.Source.ID > 0 && selected[event.Source.ID] == 0 {
			if len(selected) == 64 {
				result.SourcesLimited = true
				continue
			}
			selected[event.Source.ID] = len(selected) + 1
			result.MatchedRequests++
		}
	}
	// Follow only dependencies of matched requests, never unrelated startup
	// network traffic. Aliases are local to this projection, not native IDs.
	for changed := true; changed; {
		changed = false
		for _, event := range raw.Events {
			dependency := event.Params.Dependency
			if selected[event.Source.ID] == 0 || dependency == nil || dependency.ID <= 0 || selected[dependency.ID] != 0 {
				continue
			}
			if len(selected) == 64 {
				result.SourcesLimited = true
				continue
			}
			selected[dependency.ID] = len(selected) + 1
			changed = true
		}
	}
	result.Sources = len(selected)
	names := map[int]string{}
	for name, number := range raw.Constants.EventTypes {
		switch name {
		case "CANCELLED", "FAILED", "URL_REQUEST_START_JOB", "URL_REQUEST_ALIVE", "URL_REQUEST_DELEGATE_CONNECTED", "URL_REQUEST_DELEGATE_RESPONSE_STARTED", "HTTP_STREAM_REQUEST", "HTTP_STREAM_JOB", "TCP_CONNECT", "TCP_CONNECT_ATTEMPT", "SOCKET_ALIVE":
			names[number] = name
		}
	}
	var origin int64
	for _, event := range raw.Events {
		alias := selected[event.Source.ID]
		if alias == 0 {
			continue
		}
		if len(result.Events) == 256 {
			result.Dropped++
			continue
		}
		stamp, err := strconv.ParseInt(event.Time, 10, 64)
		if err != nil || stamp < 0 || stamp > 1<<53 {
			return nil, fmt.Errorf("invalid network diagnostic time")
		}
		if len(result.Events) == 0 {
			origin = stamp
		}
		name := names[event.Type]
		if name == "" {
			name = "other"
		}
		row := firstNavigationNetEvent{Source: alias, SourceType: event.Source.Type, Type: event.Type, Name: name, Phase: event.Phase, Time: stamp - origin}
		if event.Params.Dependency != nil {
			row.Dependency = selected[event.Params.Dependency.ID]
		}
		if event.Params.NetError != nil && *event.Params.NetError >= -100000 && *event.Params.NetError <= 100000 {
			row.NetError = event.Params.NetError
		}
		result.Events = append(result.Events, row)
	}
	return result, nil
}

func TestFirstNavigationNetLogProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netlog.json")
	raw := `{"constants":{"logEventTypes":{"URL_REQUEST_START_JOB":1,"SECRET_PRIVATE":2,"CANCELLED":3}},"events":[
{"source":{"id":900,"type":1},"time":"1000","type":1,"phase":1,"params":{"url":"http://fixture.invalid/check","headers":"secret-cookie","source_dependency":{"id":901,"type":2}}},
{"source":{"id":901,"type":2},"time":"1001","type":2,"phase":0,"params":{"address":"private-address","net_error":-3}},
{"source":{"id":900,"type":1},"time":"1002","type":3,"phase":0,"params":{"reason":"secret-reason","net_error":-3}},
{"source":{"id":999,"type":1},"time":"1003","type":1,"phase":1,"params":{"url":"https://unrelated.invalid/secret"}}]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readFirstNavigationNetLog(path, "http://fixture.invalid/check")
	if err != nil || result.MatchedRequests != 1 || result.Sources != 2 || len(result.Events) != 3 || result.Events[1].Name != "other" || result.Events[0].Dependency != 2 || *result.Events[2].NetError != -3 {
		t.Fatal("request dependency projection failed", result, err)
	}
	encoded, _ := json.Marshal(result)
	if result.Events[0].Source != 1 || result.Events[1].Source != 2 {
		t.Fatal("source IDs were not normalized")
	}
	for _, secret := range []string{"secret", "SECRET", "http", "private"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("private metadata escaped projection: %q", secret)
		}
	}
	empty, err := readFirstNavigationNetLog(path, "http://other.invalid/check")
	if err != nil || empty.MatchedRequests != 0 || len(empty.Events) != 0 {
		t.Fatal("unrelated traffic selected", empty, err)
	}
}

func TestFirstNavigationNetLogBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netlog.json")
	var rows []string
	for i := 1; i <= 300; i++ {
		rows = append(rows, fmt.Sprintf(`{"source":{"id":%d,"type":1},"type":1,"time":"1000","params":{"url":"fixture","source_dependency":{"id":%d,"type":1}}}`, i, i+1))
	}
	for i := 0; i < 300; i++ {
		rows = append(rows, `{"source":{"id":1,"type":1},"type":1,"time":"1000","params":{"net_error":-3}}`)
	}
	if err := os.WriteFile(path, []byte(`{"events":[`+strings.Join(rows, ",")+`]}`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := readFirstNavigationNetLog(path, "fixture")
	if err != nil || result.Sources != 64 || !result.SourcesLimited || len(result.Events) != 256 || result.Dropped == 0 {
		t.Fatal("projection bounds not enforced", result, err)
	}
	for _, invalid := range []string{`{"events":[`, strings.Repeat(" ", 8*1024*1024+1), `{"events":[{"source":{"id":1},"time":"NaN","params":{"url":"fixture"}}]}`} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readFirstNavigationNetLog(path, "fixture"); err == nil {
			t.Fatal("invalid or oversized input accepted")
		}
	}
}
