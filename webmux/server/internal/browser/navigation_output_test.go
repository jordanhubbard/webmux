package browser

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// Anonymous fixture browsers only. Never retain raw browser output in an
// artifact: reduce the existing stream to a fixed, source-verified signal.
type firstNavigationOutput struct {
	ExecutableDeniedReports         int   `json:"sandboxExecutableAccessDeniedReports"`
	StartupExecutableDenied         int   `json:"startupExecutableDeniedReports"`
	NavigationExecutableDenied      int   `json:"navigationExecutableDeniedReports"`
	AfterNavigationExecutableDenied int   `json:"afterNavigationExecutableDeniedReports"`
	Bytes                           int64 `json:"bytes"`
	Lines                           int   `json:"lines"`
	DroppedBytes                    int64 `json:"droppedBytes"`
	DroppedLines                    int   `json:"droppedLines"`
	PendingBytes                    int   `json:"pendingBytes"`
	RestartReports                  int   `json:"networkServiceRestartReports"`
	StartupReports                  int   `json:"startupReports"`
	NavigationReports               int   `json:"navigationReports"`
	AfterNavigationReports          int   `json:"afterNavigationReports"`
}

type firstNavigationOutputCapture struct {
	mu       sync.Mutex
	output   firstNavigationOutput
	line     []byte
	skipping bool
	phase    string
}

func (c *firstNavigationOutputCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	remaining := max(int64(256*1024)-c.output.Bytes, 0)
	scan := min(int64(len(p)), remaining)
	c.output.Bytes += int64(len(p))
	c.output.DroppedBytes += int64(len(p)) - scan
	for _, b := range p[:scan] {
		if b == '\n' {
			c.output.Lines++
			if !c.skipping {
				c.observeLine()
			}
			c.line = c.line[:0]
			c.skipping = false
			continue
		}
		if c.skipping {
			continue
		}
		if len(c.line) == 4096 {
			c.output.DroppedLines++
			c.skipping = true
			c.line = c.line[:0]
			continue
		}
		c.line = append(c.line, b)
	}
	// Output observation must never stop the allocator draining the pipe.
	return len(p), nil
}

func (c *firstNavigationOutputCapture) observeLine() {
	line := string(c.line)
	if !strings.Contains(line, "ERROR:") {
		return
	}
	if strings.Contains(line, "network_service_instance_impl.cc:") && strings.Contains(line, "Network service crashed or was terminated, restarting service.") {
		c.output.RestartReports++
		switch c.phase {
		case "navigation":
			c.output.NavigationReports++
		case "after_navigation":
			c.output.AfterNavigationReports++
		default:
			c.output.StartupReports++
		}
	}
	if strings.Contains(line, "sandbox_win.cc:") && strings.Contains(line, "Sandbox cannot access executable ") && strings.Contains(line, ". Check filesystem permissions are valid. See https://bit.ly/31yqMJR.") {
		c.output.ExecutableDeniedReports++
		switch c.phase {
		case "navigation":
			c.output.NavigationExecutableDenied++
		case "after_navigation":
			c.output.AfterNavigationExecutableDenied++
		default:
			c.output.StartupExecutableDenied++
		}
	}
}

func (c *firstNavigationOutputCapture) setPhase(phase string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.phase = phase
}

func (c *firstNavigationOutputCapture) snapshot() firstNavigationOutput {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := c.output
	result.PendingBytes = len(c.line)
	return result
}

func TestFirstNavigationOutputBoundedSignal(t *testing.T) {
	c := &firstNavigationOutputCapture{}
	line := "[123:456:ERROR:content/browser/network_service_instance_impl.cc:655] Network service crashed or was terminated, restarting service. secret-url-and-headers\n"
	for _, part := range []string{line[:31], line[31:]} {
		n, err := c.Write([]byte(part))
		if n != len(part) || err != nil {
			t.Fatal("writer blocked output")
		}
	}
	c.setPhase("navigation")
	c.Write([]byte(line))
	c.setPhase("after_navigation")
	c.Write([]byte(line))
	c.Write([]byte("Network service crashed or was terminated, restarting service.\n"))
	c.Write([]byte("[ERROR:unrelated.cc:1] Network service crashed or was terminated, restarting service.\n"))
	got := c.snapshot()
	if got.RestartReports != 3 || got.StartupReports != 1 || got.NavigationReports != 1 || got.AfterNavigationReports != 1 || got.Lines != 5 || got.PendingBytes != 0 {
		t.Fatal("fixed source signal or phase attribution incorrect")
	}
	encoded, err := json.Marshal(got)
	if err != nil || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "network_service_instance_impl") {
		t.Fatal("raw output escaped projection")
	}
	c.Write([]byte(strings.Repeat("x", 4097) + line))
	if got := c.snapshot(); got.RestartReports != 3 || got.DroppedLines != 1 {
		t.Fatal("overlong output line must be discarded")
	}
	c.Write([]byte(strings.Repeat("x", 300*1024)))
	if got := c.snapshot(); got.DroppedBytes == 0 || got.PendingBytes > 4096 {
		t.Fatal("output scan exceeded bounds")
	}
}

func TestFirstNavigationOutputSandboxExecutableAccess(t *testing.T) {
	c := &firstNavigationOutputCapture{}
	line := `[123:456:ERROR:sandbox\policy\win\sandbox_win.cc:805] Sandbox cannot access executable C:\Users\private-user\browser\chrome.exe. Check filesystem permissions are valid. See https://bit.ly/31yqMJR.: Access is denied. (0x5)` + "\n"
	c.Write([]byte(line))
	c.setPhase("navigation")
	c.Write([]byte(line[:57]))
	c.Write([]byte(line[57:]))
	c.setPhase("after_navigation")
	c.Write([]byte(line))
	c.Write([]byte(strings.Replace(line, "sandbox_win.cc:", "unrelated.cc:", 1)))
	c.Write([]byte(strings.Replace(line, "ERROR:", "INFO:", 1)))
	got := c.snapshot()
	if got.ExecutableDeniedReports != 3 || got.StartupExecutableDenied != 1 || got.NavigationExecutableDenied != 1 || got.AfterNavigationExecutableDenied != 1 || got.RestartReports != 0 || got.Lines != 5 {
		t.Fatal("sandbox access report source/severity/phases incorrect")
	}
	encoded, err := json.Marshal(got)
	if err != nil || strings.Contains(string(encoded), "private-user") || strings.Contains(string(encoded), "chrome.exe") || strings.Contains(string(encoded), "bit.ly") {
		t.Fatal("sandbox report disclosed raw output")
	}
}

type firstNavigationNetworkSandbox struct {
	Observed     bool   `json:"observed"`
	AppContainer bool   `json:"appContainer"`
	ProcessCount int    `json:"processCount"`
	ErrorClass   string `json:"errorClass,omitempty"`
}
