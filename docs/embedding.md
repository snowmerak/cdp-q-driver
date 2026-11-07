# Embed the existing CLI as `q cdp`

Import `github.com/snowmerak/cdp-q-driver/cli` to put the complete command
interface inside another Go executable. This is the existing CLI, including
browser discovery/launch, named sessions, tab/task commands, native input,
events/files, and JavaScript `run`. It does not invoke a separately installed
`cdp-q-driver` executable or require rebuilding the SDK operations as tools.

Add a dispatch branch near the start of the host's main function, before
starting its UI or other services. In a `q` executable:

```go
import cdpcli "github.com/snowmerak/cdp-q-driver/cli"

// ctx is the host's signal-aware lifetime context.
if len(os.Args) > 1 && os.Args[1] == "cdp" {
    code := cdpcli.MainWithOptions(
        ctx, os.Args[2:], os.Stdin, os.Stdout, os.Stderr,
        cdpcli.Options{CommandPrefix: []string{"cdp"}},
    )
    os.Exit(code)
}
```

`args` excludes the executable and the `cdp` token. The prefix is supplied
separately because automatic session startup re-executes `os.Executable()`.
Without it, the child would receive `q --session ... serve`; with it, the
child correctly receives `q cdp --session ... serve`. The host must dispatch
this path non-interactively, without consuming its stdin. For a deeper
subcommand, provide all prefix tokens in order.

`MainWithOptions` returns the CLI's exit code and does not call `os.Exit` or
install signal handlers. The caller owns the context and streams. Perform any
host-specific shutdown before exiting; note that `os.Exit` skips deferred
cleanup. `cli.Main` is also available when no subcommand prefix is needed.

The host then exposes the same commands with a different executable prefix:

```sh
q cdp help
q cdp --session work page new --name main https://example.com
q cdp --session work page snapshot main
q cdp --session work --timeout 60s run workflow.js
q cdp --session work stop
```

Node.js is required only for `run`, just as with the standalone CLI. Chrome
is discovered/launched by the embedded code. The module's Go requirement still
applies to the embedding application.

Session names, OS application-data paths, runtime format, JSON envelopes,
timeouts, and cleanup rules remain unchanged. Consequently an embedded host
and the standalone CLI can reuse the same named session. Use a distinct
`--session` or `--data-dir` if the host needs separate state. Existing running
sessions keep the executable that originally started them until stopped.

## Runnable example and verification

The [example host](../examples/embedded-cli/main.go) contains the full dispatch
and signal handling. Build it with any executable name; for example on Windows:

```powershell
go build -o bin/q-cdp-example.exe ./examples/embedded-cli
.\bin\q-cdp-example.exe cdp --session demo page new --name main https://example.com
.\bin\q-cdp-example.exe cdp --session demo stop
```

The real-browser integration test builds this host and verifies first-use
background startup, cross-invocation input and reads, JavaScript execution,
stop cleanup, and restart using saved settings. The standalone executable also
uses the same public entry point.
