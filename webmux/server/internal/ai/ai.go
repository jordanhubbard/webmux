// Package ai forwards terminal chat to the configured RCC or direct provider.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/jordanhubbard/webmux/server/internal/config"
)

const SystemPrompt = `You are a terminal assistant embedded in webmux, a browser-based terminal multiplexer.
You help users understand command output, diagnose errors, and suggest next steps.
When you see terminal output in <terminal_context> tags, use it to give specific, actionable advice.
Keep responses concise and focused. Prefer shell commands over lengthy explanations.
Format commands in backticks. Do not repeat the terminal context back to the user.`
const Hint = "Set WEBMUX_RCC_URL+WEBMUX_RCC_TOKEN or NVIDIA_API_KEY/OPENAI_API_KEY"
const MaxResponseBytes = 1 << 20

type Message struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	rawContent json.RawMessage
}

func (m Message) MarshalJSON() ([]byte, error) {
	if m.rawContent != nil {
		return json.Marshal(struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}{m.Role, m.rawContent})
	}
	type plain Message
	return json.Marshal(plain(m))
}

type Request struct {
	Message string    `json:"message"`
	Context string    `json:"context"`
	History []Message `json:"history"`
}
type Response struct {
	Reply     string `json:"reply"`
	Model     string `json:"model"`
	Source    string `json:"source"`
	Timestamp int64  `json:"ts"`
}
type Providers struct {
	RCC    bool `json:"rcc"`
	NVIDIA bool `json:"nvidia"`
	OpenAI bool `json:"openai"`
}
type Status struct {
	Available bool      `json:"available"`
	Providers Providers `json:"providers"`
	Model     string    `json:"model"`
}
type Service struct {
	env    func(string) string
	client *http.Client
}

func New(env func(string) string, client *http.Client) *Service {
	if env == nil {
		env = os.Getenv
	}
	if client == nil {
		client = &http.Client{CheckRedirect: func(next *http.Request, previous []*http.Request) error {
			if len(previous) >= 10 {
				return errors.New("provider redirect limit exceeded")
			}
			first := previous[0].URL
			if !strings.EqualFold(first.Scheme, next.URL.Scheme) || !strings.EqualFold(first.Host, next.URL.Host) {
				return http.ErrUseLastResponse
			}
			return nil
		}}
	}
	return &Service{env: env, client: client}
}
func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func (s *Service) rccURL() string { return first(s.env("WEBMUX_RCC_URL"), s.env("LOOM_RCC_BRAIN_URL")) }
func (s *Service) Status() Status {
	p := Providers{RCC: s.rccURL() != "", NVIDIA: s.env("NVIDIA_API_KEY") != "", OpenAI: s.env("OPENAI_API_KEY") != ""}
	model := "gpt-4o-mini"
	if p.NVIDIA {
		model = "meta/llama-3.1-70b-instruct"
	}
	return Status{Available: p.RCC || p.NVIDIA || p.OpenAI, Providers: p, Model: first(s.env("WEBMUX_MODEL"), model)}
}

func messages(input Request) []Message {
	result := []Message{{Role: "system", Content: SystemPrompt}}
	history := input.History
	if len(history) > 10 {
		history = history[len(history)-10:]
	}
	for _, message := range history {
		if message.Role != "system" {
			result = append(result, message)
		}
	}
	user := Message{Role: "user", Content: config.TrimSpace(input.Message)}
	if terminal := config.TrimSpace(input.Context); terminal != "" {
		units := utf16.Encode([]rune(terminal))
		if len(units) > 3000 {
			units = units[len(units)-3000:]
		}
		prefix, suffix := "<terminal_context>\n", "\n</terminal_context>\n\n"+user.Content
		user.Content = prefix + string(utf16.Decode(units)) + suffix
		// JavaScript slice counts UTF-16 units and can leave a leading low
		// surrogate. Preserve that escaped unit instead of replacing it with U+FFFD.
		if units[0] >= 0xdc00 && units[0] <= 0xdfff {
			a, _ := json.Marshal(prefix)
			b, _ := json.Marshal(string(utf16.Decode(units[1:])) + suffix)
			user.rawContent = []byte(string(a[:len(a)-1]) + fmt.Sprintf(`\u%04x`, units[0]) + string(b[1:]))
		}
	}
	return append(result, user)
}

func (s *Service) post(ctx context.Context, url, key string, payload any, timeout time.Duration, provider string, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return errors.New("Invalid provider request")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return errors.New("Invalid provider URL")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := s.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return errors.New("The operation was aborted due to timeout")
		}
		return errors.New("fetch failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s HTTP %d", provider, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxResponseBytes+1))
	if err != nil {
		return errors.New("Invalid provider response")
	}
	if len(body) > MaxResponseBytes {
		return errors.New("Provider response too large")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errors.New("Invalid provider response")
	}
	return nil
}

func (s *Service) rcc(ctx context.Context, messages []Message) (string, error) {
	url := s.rccURL()
	if url == "" {
		return "", errors.New("RCC not configured")
	}
	var response struct {
		Status string  `json:"status"`
		Result *string `json:"result"`
	}
	err := s.post(ctx, strings.TrimSuffix(url, "/")+"/api/brain/request", first(s.env("WEBMUX_RCC_TOKEN"), s.env("LOOM_RCC_AGENT_TOKEN")), map[string]any{
		"messages": messages, "maxTokens": 512, "priority": "normal", "metadata": map[string]string{"source": "webmux-ai"},
	}, 15*time.Second, "RCC", &response)
	if err != nil {
		return "", err
	}
	if response.Status != "completed" {
		return "", fmt.Errorf("Brain status: %s", response.Status)
	}
	if response.Result == nil {
		return "", errors.New("Invalid provider response")
	}
	return *response.Result, nil
}

func (s *Service) direct(ctx context.Context, messages []Message) (string, error) {
	nvKey, oaKey := s.env("NVIDIA_API_KEY"), s.env("OPENAI_API_KEY")
	key := first(nvKey, oaKey)
	if key == "" {
		return "", errors.New("No LLM API key configured")
	}
	url, model := "https://api.openai.com/v1/chat/completions", "gpt-4o-mini"
	if nvKey != "" {
		url, model = "https://integrate.api.nvidia.com/v1/chat/completions", "meta/llama-3.1-70b-instruct"
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	err := s.post(ctx, url, key, map[string]any{"model": first(s.env("WEBMUX_MODEL"), model), "messages": messages, "max_tokens": 512, "temperature": 0.3}, 20*time.Second, "LLM API", &response)
	if err != nil {
		return "", err
	}
	if response.Choices == nil {
		return "", errors.New("Invalid provider response")
	}
	if len(response.Choices) == 0 {
		return "", nil
	}
	return config.TrimSpace(response.Choices[0].Message.Content), nil
}

func (s *Service) Chat(ctx context.Context, input Request) (Response, error) {
	messages := messages(input)
	reply, err := s.rcc(ctx, messages)
	model, source := "rcc-brain", "rcc"
	if err != nil {
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		reply, err = s.direct(ctx, messages)
		if err != nil {
			return Response{}, err
		}
		model, source = first(s.env("WEBMUX_MODEL"), "llm"), "direct"
	}
	return Response{Reply: reply, Model: model, Source: source, Timestamp: time.Now().UnixMilli()}, nil
}
