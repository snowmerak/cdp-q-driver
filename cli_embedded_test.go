package qdriver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIEmbeddedSubcommandWorkflow(t *testing.T) {
	chrome := startChrome(t)
	binary := filepath.Join(t.TempDir(), "q.exe")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./examples/embedded-cli")
	hideWindow(build)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build embedded CLI: %v\n%s", err, output)
	}
	f := namedCLI(t, binary, t.TempDir(), "embedded")
	f.prefix = []string{"cdp"}
	f.ok(t, "--endpoint", chrome.endpoint, "page", "new", "--name", "main", "data:text/html,<title>Embedded CLI</title><input id=name>")
	f.ok(t, "page", "fill", "--css", "#name", "main", "embedded")
	if got := string(f.ok(t, "page", "eval", "main", "document.querySelector('input').value")); got != `"embedded"` {
		t.Fatalf("embedded input/session reuse failed: %s", got)
	}
	if _, err := exec.LookPath("node"); err == nil {
		got := string(f.ok(t, "run", scriptFile(t, `return await session.page('main').title();`)))
		if !strings.Contains(got, "Embedded CLI") {
			t.Fatalf("embedded JavaScript run failed: %s", got)
		}
	}
	f.ok(t, "stop")
	if _, err := os.Stat(f.state); !os.IsNotExist(err) {
		t.Fatalf("embedded stop retained runtime: %v", err)
	}
	// Restart from saved settings must also go through `q cdp serve`.
	f.ok(t, "session", "ensure")
	f.ok(t, "page", "new", "--name", "again")
	f.ok(t, "stop")
}
