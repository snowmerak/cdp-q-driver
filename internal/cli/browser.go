package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto"
	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp/remote"
)

// Only the session process owns this child. External --endpoint browsers never
// enter this lifecycle, including its Browser.close and process-kill fallback.
type launchedBrowser struct {
	cmd                 *exec.Cmd
	done                chan struct{}
	profile, lock       string
	endpoint, websocket string
	log                 *os.File
	once                sync.Once
	closeErr            error
}

func launchBrowser(ctx context.Context, o appOptions, serve serveOptions) (_ *launchedBrowser, resultErr error) {
	profile := filepath.Join(filepath.Dir(o.State), "chrome-profile")
	if err := os.MkdirAll(profile, 0700); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(filepath.Dir(o.State), "browser.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil, &commandError{Code: "browser_in_use", Message: "browser.lock exists; inspect the previous session and confirm its Chrome process has exited before removing that exact lock"}
	}
	if err != nil {
		return nil, err
	}
	_, writeErr := fmt.Fprintf(lock, "controlPid=%d\n", os.Getpid())
	if err := errors.Join(writeErr, lock.Close()); err != nil {
		_ = os.Remove(lockPath)
		return nil, err
	}
	owned := &launchedBrowser{profile: profile, lock: lockPath}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, owned.close())
		}
	}()
	// The driver lock prevents normal concurrent launches and leaves a visible
	// recovery marker after an abrupt crash. Never reuse stale endpoint data.
	activePort := filepath.Join(profile, "DevToolsActivePort")
	if err := os.Remove(activePort); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	owned.log, err = os.CreateTemp(filepath.Dir(o.State), "chrome-*.log")
	if err != nil {
		return nil, err
	}
	args := []string{"--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0",
		"--user-data-dir=" + profile, "--no-first-run", "--no-default-browser-check"}
	if serve.Headless {
		args = append(args, "--headless=new")
	}
	args = append(args, "about:blank")
	owned.cmd = exec.Command(serve.Chrome, args...)
	browserProcess(owned.cmd, serve.Headless)
	owned.cmd.Stdout, owned.cmd.Stderr = owned.log, owned.log
	if err := owned.cmd.Start(); err != nil {
		return nil, fmt.Errorf("launch Chrome: %w", err)
	}
	owned.done = make(chan struct{})
	go func() { _ = owned.cmd.Wait(); close(owned.done) }()
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("controlPid=%d\nbrowserPid=%d\n", os.Getpid(), owned.cmd.Process.Pid)), 0600); err != nil {
		return nil, err
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if endpoint, websocket, err := readActivePort(activePort); err == nil {
			owned.endpoint, owned.websocket = endpoint, websocket
			probe, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			conn, dialErr := remote.DialContext(probe, websocket)
			cancel()
			if dialErr == nil {
				_ = conn.Close()
				select {
				case <-owned.done:
					return nil, fmt.Errorf("browser exited during startup; inspect %s", owned.log.Name())
				default:
					return owned, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("browser readiness (inspect %s): %w", owned.log.Name(), ctx.Err())
		case <-owned.done:
			return nil, fmt.Errorf("browser exited before CDP readiness; inspect %s", owned.log.Name())
		case <-ticker.C:
		}
	}
}

func readActivePort(path string) (endpoint, websocket string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return "", "", err
	}
	lines := strings.Fields(string(data))
	if len(data) > 4096 || len(lines) != 2 {
		return "", "", errors.New("invalid DevToolsActivePort file")
	}
	port, err := strconv.Atoi(lines[0])
	if err != nil || port < 1 || port > 65535 || !strings.HasPrefix(lines[1], "/devtools/browser/") || strings.ContainsAny(lines[1], "?#\\") || strings.Contains(lines[1], "..") {
		return "", "", errors.New("invalid DevToolsActivePort endpoint")
	}
	host := "127.0.0.1:" + strconv.Itoa(port)
	return "http://" + host, "ws://" + host + lines[1], nil
}

func (b *launchedBrowser) info() map[string]any {
	return map[string]any{"managed": true, "pid": b.cmd.Process.Pid, "profile": b.profile, "endpoint": b.endpoint}
}

func (b *launchedBrowser) close() error {
	b.once.Do(func() {
		if b.done != nil {
			select {
			case <-b.done:
			default:
				// A separate connection targets only this freshly launched browser.
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				if b.websocket != "" {
					if conn, err := remote.DialContext(ctx, b.websocket); err == nil {
						_ = conn.Write(ctx, &cdproto.Message{ID: 1, Method: browser.CommandClose})
						_ = conn.Close()
					}
				}
				cancel()
				select {
				case <-b.done:
				case <-time.After(2 * time.Second):
					killErr := b.cmd.Process.Kill()
					select {
					case <-b.done:
					case <-time.After(2 * time.Second):
						b.closeErr = errors.Join(killErr, errors.New("owned Chrome did not exit; browser.lock retained"))
					}
				}
			}
		}
		if b.closeErr == nil {
			b.closeErr = os.Remove(b.lock)
		}
		if b.log != nil {
			b.closeErr = errors.Join(b.closeErr, b.log.Close())
		}
	})
	return b.closeErr
}
