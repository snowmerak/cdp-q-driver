package qdriver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
	qdriver "github.com/snowmerak/cdp-q-driver"
)

type chromeFixture struct {
	endpoint  string
	websocket string
	control   *chromedp.Browser
	cmd       *exec.Cmd
	exited    chan struct{}
	conn      *remote.Conn
	stop      context.CancelFunc
}

func chromeExecutable(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("QDRIVER_CHROME"); path != "" {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("QDRIVER_CHROME: %v", err)
		}
		return path
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome", "msedge"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
			if root != "" {
				candidates = append(candidates, filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
					filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
	case "darwin":
		candidates = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"}
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if os.Getenv("QDRIVER_INTEGRATION") == "1" {
		t.Fatal("Chromium is required; set QDRIVER_CHROME to its executable")
	}
	t.Skip("Chromium not found; set QDRIVER_CHROME (QDRIVER_INTEGRATION=1 makes this mandatory)")
	return ""
}

func startChrome(t *testing.T) *chromeFixture {
	return startChromeWithArgs(t)
}

func startChromeWithArgs(t *testing.T, extra ...string) *chromeFixture {
	t.Helper()
	executable := chromeExecutable(t)
	profile := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "chrome.log")
	output, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = output.Close() })
	args := []string{"--headless=new", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0",
		"--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check", "--disable-background-networking"}
	args = append(args, extra...)
	args = append(args, "about:blank")
	cmd := exec.Command(executable, args...)
	hideWindow(cmd)
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	f := &chromeFixture{cmd: cmd, exited: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(f.exited) }()
	t.Cleanup(func() {
		if f.conn != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			// This connection belongs only to this fixture. chromedp guards
			// Browser.close in Browser.Call; use the transport for fixture shutdown.
			_ = f.conn.Write(ctx, &cdproto.Message{ID: -1, Method: browser.CommandClose})
			cancel()
		}
		select {
		case <-f.exited:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill() // Only this fixture's process, never a process-name match.
			select {
			case <-f.exited:
			case <-time.After(5 * time.Second):
				t.Error("fixture browser did not exit")
			}
		}
		if f.stop != nil {
			f.stop()
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
		if err == nil {
			lines := strings.Fields(string(data))
			if len(lines) >= 2 {
				f.endpoint = "http://127.0.0.1:" + lines[0]
				f.websocket = "ws://127.0.0.1:" + lines[0] + lines[1]
				conn, err := remote.DialContext(ctx, f.websocket)
				if err != nil {
					t.Fatal(err)
				}
				lifetime, stop := context.WithCancel(context.Background())
				f.conn, f.stop = conn, stop
				f.control, err = chromedp.NewBrowserTransport(lifetime, conn, chromedp.WithBrowserLogf(func(string, ...any) {}))
				if err != nil {
					stop()
					_ = conn.Close()
					t.Fatal(err)
				}
				version, err := cdp.Call(ctx, f.control, browser.GetVersion, cdp.Empty{})
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("browser=%s protocol=%s Go=%s/%s", version.Product, version.ProtocolVersion, runtime.Version(), runtime.GOARCH)
				return f
			}
		}
		select {
		case <-ctx.Done():
			log, _ := os.ReadFile(logPath)
			t.Fatalf("browser readiness: %v\n%s", ctx.Err(), log)
		case <-f.exited:
			log, _ := os.ReadFile(logPath)
			t.Fatalf("browser exited before readiness:\n%s", log)
		case <-ticker.C:
		}
	}
}

func (f *chromeFixture) pageIDs(t *testing.T) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := cdp.Call(ctx, f.control, target.GetTargets, target.GetTargetsParams{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, info := range result.TargetInfos {
		if info.Type == "page" {
			ids = append(ids, string(info.TargetID))
		}
	}
	slices.Sort(ids)
	return ids
}

func attachDriver(t *testing.T, endpoint string) *qdriver.Browser {
	t.Helper()
	b, err := qdriver.Attach(t.Context(), endpoint, qdriver.Options{CommandTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = b.Close(context.Background()) })
	return b
}

func newTask(t *testing.T, b *qdriver.Browser) *qdriver.Task {
	t.Helper()
	task, err := b.NewTask(t.Context(), t.Name())
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func newPage(t *testing.T, task *qdriver.Task) *qdriver.Page {
	t.Helper()
	p, err := task.NewPage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBrowserLifecycle(t *testing.T) {
	f := startChrome(t)
	baseline := f.pageIDs(t)
	attachCtx, cancelAttachment := context.WithCancel(t.Context())
	b, err := qdriver.Attach(attachCtx, f.endpoint, qdriver.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = b.Close(context.Background()) })
	cancelAttachment() // An attachment call's context must not own the connection.
	pages, err := b.Pages(t.Context())
	if err != nil || len(pages) != len(baseline) {
		t.Fatalf("attach changed tabs or inherited cancellation: %v, %v", pages, err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `<!doctype html><title>QDriver fixture</title><h1>CDP works</h1><input id="value">`)
	}))
	defer server.Close()
	task := newTask(t, b)
	p := newPage(t, task)
	kept := newPage(t, task)
	if err := p.Goto(t.Context(), server.URL); err != nil {
		t.Fatal(err)
	}
	if title, err := p.Title(t.Context()); err != nil || title != "QDriver fixture" {
		t.Fatalf("title=%q err=%v", title, err)
	}
	if url, err := p.URL(t.Context()); err != nil || url != server.URL+"/" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if value, err := p.Evaluate(t.Context(), `Promise.resolve({answer: 42})`); err != nil || string(value) != `{"answer":42}` {
		t.Fatalf("evaluation=%s err=%v", value, err)
	}
	if _, err := p.Evaluate(t.Context(), `throw new Error('fixture exception')`); err == nil {
		t.Fatal("JavaScript exception was ignored")
	}
	screenshot, err := p.Screenshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if image, err := png.Decode(bytes.NewReader(screenshot)); err != nil || image.Bounds().Empty() {
		t.Fatalf("invalid screenshot: %v", err)
	}
	if err := p.Activate(t.Context()); err != nil {
		t.Fatal(err)
	}
	borrowed, err := task.Adopt(t.Context(), baseline[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := borrowed.Title(t.Context()); err != nil { // Actually attach its session before releasing it.
		t.Fatal(err)
	}
	other := newTask(t, b)
	if _, err := other.Adopt(t.Context(), borrowed.ID()); !errors.Is(err, qdriver.ErrPageInUse) {
		t.Fatalf("duplicate adoption: %v", err)
	}
	if err := borrowed.Close(t.Context()); !errors.Is(err, qdriver.ErrNotOwned) {
		t.Fatalf("borrowed page close: %v", err)
	}
	if err := task.Release(t.Context(), borrowed); err != nil {
		t.Fatal(err)
	}
	if _, err := borrowed.Title(t.Context()); !errors.Is(err, qdriver.ErrPageReleased) {
		t.Fatalf("released handle remained usable: %v", err)
	}
	if _, err := task.Adopt(t.Context(), baseline[0]); err != nil {
		t.Fatal(err)
	}
	finishContext, cancelFinish := context.WithCancel(t.Context())
	cancelFinish()
	result, err := task.Finish(finishContext, kept)
	if err != nil || !slices.Contains(result.Closed, p.ID()) || !slices.Contains(result.Kept, kept.ID()) || !slices.Contains(result.Released, baseline[0]) {
		t.Fatalf("finish=%+v err=%v", result, err)
	}
	if _, err := task.NewPage(t.Context()); !errors.Is(err, qdriver.ErrTaskFinished) {
		t.Fatalf("finished task admitted work: %v", err)
	}
	again, err := task.Finish(t.Context())
	if err != nil || !slices.Equal(again.Closed, result.Closed) {
		t.Fatalf("repeat finish=%+v err=%v", again, err)
	}
	if _, err := other.Adopt(t.Context(), kept.ID()); err != nil {
		t.Fatal(err)
	}
	owned := newPage(t, other)
	if err := owned.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := owned.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	remainingOwned := newPage(t, other)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	closed, err := b.Close(canceled)
	if err != nil || !slices.Contains(closed.Closed, remainingOwned.ID()) {
		t.Fatalf("disconnect cleanup=%+v err=%v", closed, err)
	}
	want := append(slices.Clone(baseline), kept.ID())
	slices.Sort(want)
	if got := f.pageIDs(t); !slices.Equal(got, want) {
		t.Fatalf("surviving tabs=%v want=%v", got, want)
	}
	if _, err := b.Pages(t.Context()); !errors.Is(err, qdriver.ErrBrowserClosed) {
		t.Fatalf("closed connection admitted work: %v", err)
	}
}

func TestCancellationAndClose(t *testing.T) {
	f := startChrome(t)
	b := attachDriver(t, f.endpoint)
	task := newTask(t, b)
	p := newPage(t, task)
	if err := p.Goto(t.Context(), "about:blank"); err != nil {
		t.Fatal(err)
	}
	before := f.pageIDs(t)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := task.NewPage(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled new page: %v", err)
	}
	if got := f.pageIDs(t); !slices.Equal(got, before) {
		t.Fatalf("canceled call created a tab: %v", got)
	}
	short, stop := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer stop()
	_, err := p.Evaluate(short, `window.count=(window.count||0)+1;new Promise(resolve=>setTimeout(()=>resolve(window.count),500))`)
	var operationErr *qdriver.OperationError
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &operationErr) || !operationErr.MayHaveEffects {
		t.Fatalf("evaluation timeout lost cause/effect information: %v", err)
	}
	if value, err := p.Evaluate(t.Context(), "window.count"); err != nil || string(value) != "1" {
		t.Fatalf("timeout closed the tab or retried the expression: value=%s err=%v", value, err)
	}
	started := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/signal" {
			once.Do(func() { close(started) })
		}
		_, _ = fmt.Fprint(w, "started")
	}))
	defer server.Close()
	if err := p.Goto(t.Context(), server.URL); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		_, err := p.Evaluate(t.Context(), "fetch('/signal');new Promise(()=>{})")
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("pending evaluation did not start")
	}
	if _, err := b.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, qdriver.ErrBrowserClosed) {
			t.Fatalf("interrupted evaluation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not interrupt a pending call")
	}
	if slices.Contains(f.pageIDs(t), p.ID()) {
		t.Fatal("Close left its owned tab open")
	}
}

func TestConnectionLoss(t *testing.T) {
	f := startChrome(t)
	b := attachDriver(t, f.endpoint)
	p := newPage(t, newTask(t, b))
	if err := p.Goto(t.Context(), "about:blank"); err != nil {
		t.Fatal(err)
	}
	if err := f.conn.Write(t.Context(), &cdproto.Message{ID: -1, Method: browser.CommandClose}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture browser did not exit")
	}
	if _, err := b.Pages(t.Context()); !errors.Is(err, qdriver.ErrConnectionLost) {
		t.Fatalf("connection loss not identified: %v", err)
	}
	result, err := b.Close(t.Context())
	if err == nil || !slices.Contains(result.Remaining, p.ID()) {
		t.Fatalf("unconfirmed cleanup reported success: %+v, %v", result, err)
	}
}

func TestConcurrentPages(t *testing.T) {
	f := startChrome(t)
	baseline := f.pageIDs(t)
	b := attachDriver(t, f.websocket) // Exercise the full browser WebSocket path.
	results := make(chan error, 4)
	for i := range 4 {
		go func() {
			results <- func() error {
				task, err := b.NewTask(t.Context(), fmt.Sprintf("worker-%d", i))
				if err != nil {
					return err
				}
				p, err := task.NewPage(t.Context())
				if err != nil {
					return err
				}
				if err := p.Goto(t.Context(), "data:text/html,<title>Concurrent fixture</title>"); err != nil {
					return err
				}
				if title, err := p.Title(t.Context()); err != nil || title != "Concurrent fixture" {
					return fmt.Errorf("concurrent title=%q err=%v", title, err)
				}
				_, err = task.Finish(t.Context())
				return err
			}()
		}()
	}
	for range 4 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if got := f.pageIDs(t); !slices.Equal(got, baseline) {
		t.Fatalf("concurrent tasks left tabs behind: got=%v baseline=%v", got, baseline)
	}
}

func TestTabsExample(t *testing.T) {
	f := startChrome(t)
	baseline := f.pageIDs(t)
	output := runExample(t, f, "tabs")
	var result struct {
		PageID  string
		Title   string
		Cleanup qdriver.FinishResult
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode example output: %v\n%s", err, output)
	}
	if result.Title != "QDriver demo" || result.PageID == "" || !slices.Contains(result.Cleanup.Closed, result.PageID) {
		t.Fatalf("unexpected example result: %+v", result)
	}
	if got := f.pageIDs(t); !slices.Equal(got, baseline) {
		t.Fatalf("example changed external tabs: got=%v baseline=%v", got, baseline)
	}
	t.Logf("example output: %s", output)
}

func TestInteractionExample(t *testing.T) {
	f := startChrome(t)
	baseline := f.pageIDs(t)
	output := runExample(t, f, "interact")
	var result struct {
		Snapshot string
		Result   string
		Cleanup  qdriver.FinishResult
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode interaction example output: %v\n%s", err, output)
	}
	if result.Result != "Go CDP works" || !strings.Contains(result.Snapshot, "ref=@") || len(result.Cleanup.Closed) != 1 {
		t.Fatalf("unexpected interaction example output: %+v", result)
	}
	if got := f.pageIDs(t); !slices.Equal(got, baseline) {
		t.Fatalf("interaction example changed external tabs: got=%v baseline=%v", got, baseline)
	}
	t.Logf("interaction example result: %s", result.Result)
}

func runExample(t *testing.T, f *chromeFixture, name string) []byte {
	t.Helper()
	binary := filepath.Join(t.TempDir(), name+".exe")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./examples/"+name)
	hideWindow(build)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build example: %v\n%s", err, output)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-endpoint", f.endpoint)
	hideWindow(command)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run example: %v\n%s", err, output)
	}
	return output
}

func TestFramesExample(t *testing.T) {
	f := startChrome(t)
	baseline := f.pageIDs(t)
	output := runExample(t, f, "frames")
	var result struct {
		Frames  []qdriver.FrameInfo
		Result  string
		Cleanup qdriver.FinishResult
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode frames example: %v\n%s", err, output)
	}
	if result.Result != "Go frames work" || len(result.Frames) != 2 || len(result.Cleanup.Closed) != 1 {
		t.Fatalf("unexpected frames example result: %+v", result)
	}
	if got := f.pageIDs(t); !slices.Equal(got, baseline) {
		t.Fatalf("frames example left tabs behind: %v", got)
	}
	t.Logf("frames example result: %s", result.Result)
}
