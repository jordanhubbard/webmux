package nativeprovider_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/jordanhubbard/webmux/server/internal/nativeprovider"
	"github.com/jordanhubbard/webmux/server/internal/terminal"
	"golang.org/x/sys/windows"
)

func fixtureFrame(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	out := make([]byte, 4+len(data))
	binary.BigEndian.PutUint32(out, uint32(len(data)))
	copy(out[4:], data)
	return out
}
func readFixtureFrame(r io.Reader, value any) error {
	var h [4]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(h[:])
	if n > 256<<10 {
		return fmt.Errorf("oversized fixture frame")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

type controllerReply struct {
	Identity nativeprovider.Identity
	Error    string
}

func TestNativeProviderHelper(t *testing.T) {
	if os.Getenv("WEBMUX_TEST_NATIVE_PROVIDER_HELPER") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	if mode == "--native-provider-bootstrap" || mode == "--native-provider-owner" {
		var output io.Writer = os.Stdout
		if mode == "--native-provider-bootstrap" && os.Getenv("WEBMUX_TEST_DROP_PROVIDER_REPLY") == "1" {
			output = io.Discard
		}
		if err := terminal.RunNativeProviderHelper(context.Background(), mode, os.Stdin, output); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
	if mode != "--fixture-controller" && mode != "--fixture-restrictive-controller" {
		return
	}
	var cfg nativeprovider.Config
	if err := readFixtureFrame(os.Stdin, &cfg); err != nil {
		os.Exit(3)
	}
	// Only this disposable subprocess joins the test job. Its handle is left
	// open until process exit, which must kill members that did not break away.
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		os.Exit(4)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if mode == "--fixture-controller" {
		limits.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		os.Exit(5)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		os.Exit(6)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	i, _, err := nativeprovider.Ensure(ctx, cfg)
	reply := controllerReply{Identity: i}
	if err != nil {
		reply.Error = err.Error()
	}
	_, _ = os.Stdout.Write(fixtureFrame(reply))
	if mode == "--fixture-restrictive-controller" {
		os.Exit(0)
	}
	for {
		time.Sleep(time.Second)
	}
}

func permitBreakaway(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "cmd.exe", "/d", "/c", "exit", "0")
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_BREAKAWAY_FROM_JOB}
	if err := c.Run(); err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			t.Skip("outer runner job forbids independent provider processes; restrictive-job refusal is tested separately")
		}
		t.Fatal(err)
	}
}

func randomHex() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func config(t *testing.T) (nativeprovider.Config, string) {
	t.Helper()
	t.Setenv("WEBMUX_TEST_NATIVE_PROVIDER_HELPER", "1")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for real ConPTY provider qualification")
	}
	base := t.TempDir()
	counter := filepath.Join(base, "starts.log")
	script := filepath.Join(base, "job.cjs")
	if err := os.WriteFile(script, []byte(`const fs=require('fs');
fs.appendFileSync(process.argv[2],process.pid+'\n');
process.stdin.setRawMode(true);process.stdin.resume();
process.stdin.on('data',d=>process.stdout.write('INPUT:'+d.toString('hex')+';'));
let pending='';process.stdin.on('data',d=>{pending=(pending+d.toString()).slice(-256);if(pending.includes('SIZE?')){pending='';process.stdout.write('SIZE:'+process.stdout.getWindowSize().join('x')+';');}});
process.stdout.write('JOB_READY:'+process.pid+';');
`), 0600); err != nil {
		t.Fatal(err)
	}
	x := randomHex()
	session := x[:8] + "-" + x[8:12] + "-" + x[12:16] + "-" + x[16:20] + "-" + x[20:]
	c := nativeprovider.Config{Root: filepath.Join(base, "native-providers", session), Session: session, Key: randomHex(), Executable: self, DriverArgs: []string{"-test.run=^TestNativeProviderHelper$", "--"}, Launch: nativeprovider.Launch{Program: node, Args: []string{script, counter}, Directory: base, Environment: os.Environ(), Cols: 100, Rows: 30}}
	t.Logf("provider_fixture_root=%s", c.Root)
	// Cover a lost readiness response before the normal owner cleanup can be
	// registered. This reads only the test's fresh private identity directory.
	t.Cleanup(func() {
		data, err := os.ReadFile(filepath.Join(c.Root, "identity.json"))
		if err != nil {
			return
		}
		var saved struct{ Identity nativeprovider.Identity }
		if json.Unmarshal(data, &saved) != nil || saved.Identity.Session != c.Session {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := nativeprovider.Probe(ctx, saved.Identity, c.Key); err == nil {
			_ = nativeprovider.Stop(ctx, saved.Identity, c.Key)
		}
		h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(saved.Identity.OwnerPID))
		if err == nil {
			defer windows.CloseHandle(h)
			if status, _ := windows.WaitForSingleObject(h, 10000); status != windows.WAIT_OBJECT_0 {
				t.Error("partially started fixture owner did not exit")
			}
		}
	})
	return c, counter
}

func ensure(t *testing.T, c nativeprovider.Config) nativeprovider.Identity {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	i, _, err := nativeprovider.Ensure(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func processHandle(t *testing.T, pid int) windows.Handle {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
	return h
}
func awaitExit(t *testing.T, h windows.Handle) {
	t.Helper()
	status, err := windows.WaitForSingleObject(h, 10000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("owned fixture did not exit: %d, %v", status, err)
	}
}

func cleanupOwner(t *testing.T, c nativeprovider.Config, i nativeprovider.Identity) {
	t.Helper()
	h := processHandle(t, i.OwnerPID)
	t.Cleanup(func() {
		status, _ := windows.WaitForSingleObject(h, 0)
		if status == windows.WAIT_OBJECT_0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = nativeprovider.Stop(ctx, i, c.Key)
		cancel()
		if status, _ := windows.WaitForSingleObject(h, 10000); status != windows.WAIT_OBJECT_0 {
			_ = windows.TerminateProcess(h, 1)
			_, _ = windows.WaitForSingleObject(h, 10000)
			t.Error("owned provider required forced fixture cleanup")
		}
	})
}

type capture struct {
	sync.Mutex
	data strings.Builder
}

func (c *capture) Write(b []byte) (int, error) { c.Lock(); defer c.Unlock(); return c.data.Write(b) }
func (c *capture) text() string                { c.Lock(); defer c.Unlock(); return c.data.String() }
func attach(t *testing.T, c nativeprovider.Config, i nativeprovider.Identity) (*nativeprovider.Client, *capture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p, err := nativeprovider.Attach(ctx, i, c.Key, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	out := &capture{}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(out, p); close(done) }()
	t.Cleanup(func() {
		_ = p.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("viewer drain did not stop")
		}
	})
	return p, out
}
func expect(t *testing.T, out *capture, text string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !strings.Contains(out.text(), text) {
		if time.Now().After(deadline) {
			t.Fatalf("missing %q in provider output: %q", text, out.text())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func starts(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(string(b)))
}

func expectInput(t *testing.T, out *capture, want string) {
	t.Helper()
	expected := hex.EncodeToString([]byte(want))
	deadline := time.Now().Add(8 * time.Second)
	for {
		var joined strings.Builder
		for _, m := range regexp.MustCompile(`INPUT:([0-9a-f]+);`).FindAllStringSubmatch(out.text(), -1) {
			joined.WriteString(m[1])
		}
		if strings.Contains(joined.String(), expected) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("native input %q did not arrive: %q", want, out.text())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestNativeProviderConcurrentCreationLostReplyAndUncertainState(t *testing.T) {
	permitBreakaway(t)
	c, counter := config(t)
	t.Setenv("WEBMUX_TEST_DROP_PROVIDER_REPLY", "1")
	var group sync.WaitGroup
	results := make(chan nativeprovider.Identity, 5)
	failures := make(chan error, 5)
	for n := 0; n < 5; n++ {
		group.Add(1)
		go func() {
			defer group.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			i, _, err := nativeprovider.Ensure(ctx, c)
			if err != nil {
				failures <- err
			} else {
				results <- i
			}
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	var original nativeprovider.Identity
	for i := range results {
		if original.Instance == "" {
			original = i
			cleanupOwner(t, c, i)
		} else if i.Instance != original.Instance || i.RootPID != original.RootPID {
			t.Fatal("concurrent creation duplicated the ConPTY job")
		}
	}
	for err := range failures {
		t.Error(err)
	}
	if original.Instance == "" {
		t.Fatal("no owner was created")
	}
	p, out := attach(t, c, original)
	expect(t, out, fmt.Sprintf("JOB_READY:%d;", original.RootPID))
	t.Logf("provider_lifecycle=created owner=%d root=%d instance=%s", original.OwnerPID, original.RootPID, original.Instance)
	if err := p.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	resizeDeadline := time.Now().Add(8 * time.Second)
	for !strings.Contains(out.text(), "SIZE:120x40;") {
		if _, err := p.Write([]byte("SIZE?")); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(resizeDeadline) {
			t.Fatalf("resize did not reach the real child console: %s", out.text())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if starts(t, counter) != 1 {
		t.Fatal("lost startup reply started more than one job")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := nativeprovider.Probe(ctx, original, randomHex()); err == nil {
		t.Fatal("provider accepted another key")
	}
	if _, err := p.Write([]byte("before-detach")); err != nil {
		t.Fatal(err)
	}
	expectInput(t, out, "before-detach")
	_ = p.Close()
	i := ensure(t, c)
	if i.Instance != original.Instance || i.RootPID != original.RootPID {
		t.Fatal("viewer detach replaced the job")
	}
	_, out2 := attach(t, c, i)
	expect(t, out2, fmt.Sprintf("JOB_READY:%d;", i.RootPID))
	path := filepath.Join(c.Root, "identity.json")
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte(c.Key)) || bytes.Contains(saved, []byte("Environment")) {
		t.Fatal("credentials/launch environment were persisted")
	}
	// The lifetime mutex, not a timeout or cache file, authorizes creation.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	short, cancelShort := context.WithTimeout(context.Background(), time.Second)
	_, _, err = nativeprovider.Ensure(short, c)
	cancelShort()
	if err == nil {
		t.Fatal("missing identity record authorized a second owner")
	}
	if _, err := nativeprovider.Probe(ctx, original, c.Key); err != nil {
		t.Fatal("missing record killed the surviving job", err)
	}
	if err := os.WriteFile(path, saved, 0600); err != nil {
		t.Fatal(err)
	}
	var altered map[string]any
	if err := json.Unmarshal(saved, &altered); err != nil {
		t.Fatal(err)
	}
	altered["identity"].(map[string]any)["endpoint"] = filepath.Join(c.Root, "absent.sock")
	data, _ := json.Marshal(altered)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	short, cancelShort = context.WithTimeout(context.Background(), time.Second)
	_, _, err = nativeprovider.Ensure(short, c)
	cancelShort()
	if err == nil {
		t.Fatal("unreachable identity endpoint authorized a second job")
	}
	if starts(t, counter) != 1 {
		t.Fatal("uncertain state duplicated the job")
	}
	t.Logf("provider_lifecycle=uncertain-cache-refused owner=%d root=%d starts=1", original.OwnerPID, original.RootPID)
	if err := os.WriteFile(path, saved, 0600); err != nil {
		t.Fatal(err)
	}
}

func controller(t *testing.T, c nativeprovider.Config, mode string) (*exec.Cmd, controllerReply) {
	t.Helper()
	command := exec.Command(c.Executable, "-test.run=^TestNativeProviderHelper$", "--", mode)
	command.Stdin = bytes.NewReader(fixtureFrame(c))
	pipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait(); _ = pipe.Close() })
	reply := make(chan controllerReply, 1)
	go func() {
		var result controllerReply
		if err := readFixtureFrame(pipe, &result); err != nil {
			result.Error = err.Error()
		}
		reply <- result
	}()
	select {
	case result := <-reply:
		return command, result
	case <-time.After(20 * time.Second):
		t.Fatal("controller fixture did not report readiness")
		return nil, controllerReply{}
	}
}

func TestNativeProviderSurvivesControllerJobLossAndOwnerCrashIsExplicit(t *testing.T) {
	permitBreakaway(t)
	c, counter := config(t)
	// The controller image is separate from the test runner. It must become
	// replaceable while the provider still runs its immutable cached image.
	source, err := os.ReadFile(c.Executable)
	if err != nil {
		t.Fatal(err)
	}
	c.Executable = filepath.Join(filepath.Dir(counter), "controller.exe")
	if err := os.WriteFile(c.Executable, source, 0700); err != nil {
		t.Fatal(err)
	}
	parent, reply := controller(t, c, "--fixture-controller")
	if reply.Error != "" {
		t.Fatal(reply.Error)
	}
	i := reply.Identity
	cleanupOwner(t, c, i)
	ownerHandle := processHandle(t, i.OwnerPID)
	jobHandle := processHandle(t, i.RootPID)
	_, out := attach(t, c, i)
	expect(t, out, fmt.Sprintf("JOB_READY:%d;", i.RootPID))
	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = parent.Wait()
	if err := os.Rename(c.Executable, c.Executable+".old"); err != nil {
		t.Fatal("provider kept the controller image mapped", err)
	}
	if err := os.WriteFile(c.Executable, source, 0700); err != nil {
		t.Fatal(err)
	}
	again := ensure(t, c)
	if again.Instance != i.Instance || again.RootPID != i.RootPID {
		t.Fatal("controller/job loss replaced surviving provider")
	}
	p, out2 := attach(t, c, again)
	if _, err := p.Write([]byte("after-controller-loss")); err != nil {
		t.Fatal(err)
	}
	expectInput(t, out2, "after-controller-loss")
	if starts(t, counter) != 1 {
		t.Fatal("controller loss duplicated the job")
	}
	t.Logf("provider_lifecycle=controller-job-loss controller=%d owner=%d root=%d starts=1 image-replaceable=true", parent.Process.Pid, i.OwnerPID, i.RootPID)
	// Abrupt owner death must reap its own ConPTY job. Only a subsequent
	// explicit Ensure may create one replacement provider.
	if err := windows.TerminateProcess(ownerHandle, 1); err != nil {
		t.Fatal(err)
	}
	awaitExit(t, ownerHandle)
	awaitExit(t, jobHandle)
	replacement := ensure(t, c)
	cleanupOwner(t, c, replacement)
	if replacement.Instance == i.Instance || replacement.RootPID == i.RootPID {
		t.Fatal("dead owner identity was reused")
	}
	_, out3 := attach(t, c, replacement)
	expect(t, out3, fmt.Sprintf("JOB_READY:%d;", replacement.RootPID))
	if starts(t, counter) != 2 {
		t.Fatal("explicit replacement did not start exactly one job")
	}
	t.Logf("provider_lifecycle=explicit-replacement old-owner=%d old-root=%d owner=%d root=%d starts=2", i.OwnerPID, i.RootPID, replacement.OwnerPID, replacement.RootPID)
}

func TestNativeProviderRefusesRestrictiveControllerJob(t *testing.T) {
	c, counter := config(t)
	_, reply := controller(t, c, "--fixture-restrictive-controller")
	if !strings.Contains(reply.Error, "does not permit") {
		t.Fatalf("restrictive job was not refused: %s", reply.Error)
	}
	if _, err := os.Stat(counter); !os.IsNotExist(err) {
		t.Fatal("restrictive job started a child terminal")
	}
}

func TestNativeProviderScreenReattachmentAndContinuedInput(t *testing.T) {
	permitBreakaway(t)
	c, counter := config(t)
	if err := os.WriteFile(c.Launch.Args[0], []byte(`const fs=require('fs');
fs.appendFileSync(process.argv[2],process.pid+'\n');
process.stdin.setRawMode(true);process.stdin.resume();
process.stdin.on('data',data=>{
 const text=data.toString();
 if(text.includes('PAINT')) process.stdout.write('\x1b[?1049h\x1b[2J\x1b[H\x1b[31mHEADER_DRAWN_ONCE\x1b[0m\x1b[5;4H\x1b[32m日本語 α\x1b[18;1HBODY_READY\x1b[12;20H\x1b[36m\x1b[?25l\x1b[?2004h\x1b]52;c;c3ludGhldGlj\x07\x1b]777;webmux-browser;private-browser\x07\x1b]777;SCREEN_LIVE_BARRIER\x07');
 if(text.includes('NEXT')) process.stdout.write('AFTER_SCREEN_REATTACH');
});
process.stdout.write('PAINTER_READY:'+process.pid+';');
`), 0600); err != nil {
		t.Fatal(err)
	}
	i := ensure(t, c)
	cleanupOwner(t, c, i)
	first, original := attach(t, c, i)
	expect(t, original, "PAINTER_READY:")
	if _, err := first.Write([]byte("PAINT")); err != nil {
		t.Fatal(err)
	}
	expect(t, original, "SCREEN_LIVE_BARRIER\a")
	expect(t, original, "BODY_READY")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, restored := attach(t, c, i)
	expect(t, restored, "BODY_READY")
	view := restored.text()
	if !strings.Contains(view, "HEADER_DRAWN_ONCE") || !strings.Contains(view, "日本語") {
		t.Fatalf("reattachment lost native cells: %q", view)
	}
	if strings.Contains(view, "private-browser") || strings.Contains(view, "\x1b]52;") || strings.Contains(view, "SCREEN_LIVE_BARRIER") {
		t.Fatal("reattachment replayed a private request", view)
	}
	beforeInput := len(view)
	if _, err := second.Write([]byte("NEXT")); err != nil {
		t.Fatal(err)
	}
	expect(t, restored, "AFTER_SCREEN_REATTACH")
	if starts(t, counter) != 1 {
		t.Fatal("screen reattachment created another job")
	}
	if target := os.Getenv("WEBMUX_NATIVE_SCREEN_CAPTURE"); target != "" {
		data, err := json.Marshal(map[string]any{"cols": 100, "rows": 30, "original": original.text(), "snapshot": view, "future": restored.text()[beforeInput:], "owner_pid": i.OwnerPID, "root_pid": i.RootPID, "starts": 1})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("native_screen_replay owner=%d root=%d same_job=true snapshot_bytes=%d", i.OwnerPID, i.RootPID, len(view))
}
