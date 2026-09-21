package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(env map[string]string, f transport) *Service {
	return New(func(name string) string { return env[name] }, &http.Client{Transport: f})
}
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestRCCSuccessHistoryAndContext(t *testing.T) {
	calls := 0
	s := fixture(map[string]string{"LOOM_RCC_BRAIN_URL": "http://fixture.invalid/", "LOOM_RCC_AGENT_TOKEN": "fixture-token"}, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "http://fixture.invalid/api/brain/request" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Fatal(r.URL, "wrong authorization")
		}
		var body struct {
			Messages  []Message         `json:"messages"`
			MaxTokens int               `json:"maxTokens"`
			Priority  string            `json:"priority"`
			Metadata  map[string]string `json:"metadata"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.MaxTokens != 512 || body.Priority != "normal" || body.Metadata["source"] != "webmux-ai" {
			t.Fatal(body)
		}
		if len(body.Messages) != 11 || body.Messages[0].Content != SystemPrompt || body.Messages[1].Content != "recent" {
			t.Fatal(body.Messages)
		}
		last := body.Messages[len(body.Messages)-1]
		if last.Content != "<terminal_context>\n"+strings.Repeat("x", 3000)+"\n</terminal_context>\n\nhelp" {
			t.Fatal("context or message changed")
		}
		return response(200, `{"status":"completed","result":"  exact RCC reply  "}`), nil
	})
	history := []Message{{Role: "user", Content: "discard"}}
	for range 9 {
		history = append(history, Message{Role: "assistant", Content: "recent"})
	}
	history = append(history, Message{Role: "system", Content: "ignore"})
	got, err := s.Chat(context.Background(), Request{Message: " help ", Context: strings.Repeat("x", 3500), History: history})
	if err != nil || calls != 1 || got.Reply != "  exact RCC reply  " || got.Source != "rcc" || got.Model != "rcc-brain" || got.Timestamp <= 0 {
		t.Fatal(got, err, calls)
	}
}

func TestDirectFallbackPriorityAndDefaults(t *testing.T) {
	for _, nvidia := range []bool{false, true} {
		env := map[string]string{"WEBMUX_RCC_URL": "http://fixture.invalid", "OPENAI_API_KEY": "fixture-token"}
		wantURL, wantModel := "https://api.openai.com/v1/chat/completions", "gpt-4o-mini"
		if nvidia {
			env["NVIDIA_API_KEY"] = "nvidia-fixture"
			wantURL = "https://integrate.api.nvidia.com/v1/chat/completions"
			wantModel = "meta/llama-3.1-70b-instruct"
		}
		calls := 0
		s := fixture(env, func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return response(503, "unavailable"), nil
			}
			if r.URL.String() != wantURL {
				t.Fatal(r.URL)
			}
			key := env["OPENAI_API_KEY"]
			if nvidia {
				key = env["NVIDIA_API_KEY"]
			}
			if r.Header.Get("Authorization") != "Bearer "+key {
				t.Fatal("wrong provider key")
			}
			var body struct {
				Model       string    `json:"model"`
				MaxTokens   int       `json:"max_tokens"`
				Temperature float64   `json:"temperature"`
				Messages    []Message `json:"messages"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Model != wantModel || body.MaxTokens != 512 || body.Temperature != 0.3 || body.Messages[1].Content != "help" {
				t.Fatal(body)
			}
			return response(200, `{"choices":[{"message":{"content":"  answer\n"}}]}`), nil
		})
		got, err := s.Chat(context.Background(), Request{Message: "help"})
		if err != nil || got.Reply != "answer" || got.Source != "direct" || got.Model != "llm" || calls != 2 {
			t.Fatal(got, err, calls)
		}
		status := s.Status()
		if !status.Available || !status.Providers.RCC || !status.Providers.OpenAI || status.Providers.NVIDIA != nvidia || status.Model != wantModel {
			t.Fatal(status)
		}
	}
}

func TestProviderErrorsBoundedAndCancelled(t *testing.T) {
	for _, tc := range []struct {
		status       int
		body, detail string
	}{
		{429, "private upstream error", "LLM API HTTP 429"},
		{200, "not json", "Invalid provider response"},
		{200, `{}`, "Invalid provider response"},
		{200, strings.Repeat("x", MaxResponseBytes+1), "Provider response too large"},
	} {
		s := fixture(map[string]string{"OPENAI_API_KEY": "fixture-token"}, func(*http.Request) (*http.Response, error) { return response(tc.status, tc.body), nil })
		if _, err := s.Chat(context.Background(), Request{Message: "help"}); err == nil || err.Error() != tc.detail {
			t.Fatal(err)
		}
	}
	s := fixture(map[string]string{}, func(*http.Request) (*http.Response, error) {
		t.Fatal("unexpected network request")
		return nil, errors.New("unused")
	})
	if _, err := s.Chat(context.Background(), Request{Message: "help"}); err == nil || err.Error() != "No LLM API key configured" {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Chat(ctx, Request{Message: "help"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s = fixture(nil, func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	var out any
	if err := s.post(context.Background(), "http://fixture.invalid", "", nil, time.Millisecond, "RCC", &out); err == nil || err.Error() != "The operation was aborted due to timeout" {
		t.Fatal(err)
	}
}

func TestContextPreservesUTF16SliceBoundary(t *testing.T) {
	got := messages(Request{Message: "help", Context: "😀" + strings.Repeat("x", 2999)})
	data, err := json.Marshal(got[len(got)-1])
	if err != nil || !strings.Contains(string(data), `\ude00`) || strings.Contains(string(data), `\ufffd`) {
		t.Fatal(string(data), err)
	}
}

func TestDirectModelOverrideAndEmptyChoices(t *testing.T) {
	s := fixture(map[string]string{"OPENAI_API_KEY": "fixture-token", "WEBMUX_MODEL": "fixture-model"}, func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "fixture-model" {
			t.Fatal(body["model"])
		}
		return response(200, `{"choices":[]}`), nil
	})
	got, err := s.Chat(context.Background(), Request{Message: "help"})
	if err != nil || got.Model != "fixture-model" || got.Reply != "" || got.Source != "direct" {
		t.Fatal(got, err)
	}
}

func TestProviderRedirectDoesNotForwardCredentials(t *testing.T) {
	var forwarded atomic.Int64
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded.Add(1); w.WriteHeader(500) }))
	defer destination.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	env := map[string]string{"WEBMUX_RCC_URL": redirect.URL, "WEBMUX_RCC_TOKEN": "fixture-token"}
	s := New(func(name string) string { return env[name] }, nil)
	if _, err := s.Chat(context.Background(), Request{Message: "help"}); err == nil {
		t.Fatal("redirect accepted")
	}
	if forwarded.Load() != 0 {
		t.Fatal("redirected provider received a request")
	}
}

func TestSameOriginProviderRedirectPreservesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/brain/request" {
			http.Redirect(w, r, "/brain", http.StatusTemporaryRedirect)
			return
		}
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("redirect lost request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["priority"] != "normal" {
			t.Error("redirect lost body", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"completed","result":"redirect reply"}`)
	}))
	defer server.Close()
	env := map[string]string{"WEBMUX_RCC_URL": server.URL, "WEBMUX_RCC_TOKEN": "fixture-token"}
	s := New(func(name string) string { return env[name] }, nil)
	got, err := s.Chat(context.Background(), Request{Message: "help"})
	if err != nil || got.Reply != "redirect reply" {
		t.Fatal(got, err)
	}
}
