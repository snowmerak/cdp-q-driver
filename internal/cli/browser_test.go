package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestChromeLaunchConfigurationAndEndpointParsing(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	o := appOptions{Named: true, Session: "chrome", Metadata: filepath.Join(t.TempDir(), "session.json")}
	serve, err := resolveServe(o, serveOptions{Chrome: executable, Headless: true, headlessSet: true})
	if err != nil {
		t.Fatal(err)
	}
	serve.Endpoint = "http://127.0.0.1:12345"
	store, err := newSessionStore(o, serve, "old")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.save(t.Context()); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveServe(o, serveOptions{})
	if err != nil || resolved.Chrome != executable || resolved.Endpoint != "" || !resolved.Headless {
		t.Fatalf("saved launch settings reused old endpoint: %+v %v", resolved, err)
	}
	switched, err := resolveServe(o, serveOptions{Endpoint: "http://127.0.0.1:9222"})
	if err != nil || switched.Chrome != "" || switched.Headless {
		t.Fatalf("explicit endpoint did not select external mode: %+v %v", switched, err)
	}
	for _, requested := range []serveOptions{
		{Chrome: executable, Endpoint: "http://127.0.0.1:9222"},
	} {
		if _, err := resolveServe(o, requested); err == nil {
			t.Fatal("conflicting launch configuration accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "DevToolsActivePort")
	for _, data := range []string{"0\n/devtools/browser/id", "99999\n/devtools/browser/id", "9222\n/devtools/page/id", "9222\n//evil/id", "9222\n/devtools/browser/id?x=1", "9222", "9222\n/devtools/browser/id\nextra"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readActivePort(path); err == nil {
			t.Errorf("invalid endpoint accepted: %q", data)
		}
	}
	if err := os.WriteFile(path, []byte("45678\r\n/devtools/browser/id\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	endpoint, websocket, err := readActivePort(path)
	if err != nil || endpoint != "http://127.0.0.1:45678" || websocket != "ws://127.0.0.1:45678/devtools/browser/id" {
		t.Fatalf("invalid parsed endpoint: %s %s %v", endpoint, websocket, err)
	}
}

func TestChromeLaunchPreservesExistingLock(t *testing.T) {
	root := t.TempDir()
	lock := filepath.Join(root, "browser.lock")
	if err := os.WriteFile(lock, []byte("previous process"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := launchBrowser(context.Background(), appOptions{State: filepath.Join(root, "runtime.json")}, serveOptions{})
	if err == nil || failure(err, false).Code != "browser_in_use" {
		t.Fatalf("existing profile lock not enforced: %v", err)
	}
	data, err := os.ReadFile(lock)
	if err != nil || string(data) != "previous process" {
		t.Fatal("refused launch changed another process's lock")
	}
}
