# Command-line browser control

The CLI selects a named session with `--session ID` (default `default`) and uses
the Go SDK through that session's local control process. That process retains
tasks, page names, frame sessions, snapshot refs, and armed event subscriptions
between separate command invocations. It launches an installed Chrome executable
with `--chrome PATH`, or connects to an existing browser with `--endpoint URL`.
Session configuration, managed browser profiles, and last-observed task/page
grouping live in the OS application data directory. It does not execute
`ego-browser nodejs` scripts.

## Build and run

From the repository root on Windows:

```powershell
go build -o cdp-q-driver.exe ./cmd/cdp-q-driver
.\cdp-q-driver.exe help
.\cdp-q-driver.exe --session work page new --name main https://example.com
.\cdp-q-driver.exe --session work page snapshot main
.\cdp-q-driver.exe --session work session info
.\cdp-q-driver.exe session list
.\cdp-q-driver.exe --session work stop
```

On Linux/macOS, build with `go build -o cdp-q-driver ./cmd/cdp-q-driver`, then
use `./cdp-q-driver`. To install this checkout into Go's binary directory, run
`go install ./cmd/cdp-q-driver`. The existing module-root SDK import path stays
unchanged; the executable entry point is `cmd/cdp-q-driver`.

Without browser settings, the CLI discovers installed Chrome/Chromium on PATH
and in standard OS application locations. `QDRIVER_CHROME` can supply an
executable for discovery; `--chrome PATH` chooses one explicitly. Priority is
explicit flags, saved session configuration, `QDRIVER_CHROME`, then PATH and OS
locations. A bad explicit/saved/environment path is reported without falling
back to a different browser. Missing Chrome produces `browser_not_found` with
instructions to supply a path or an external endpoint; no runtime is created.
Discovery does not scan browser profiles or probe arbitrary debugging ports.

Managed launch supplies a dedicated persistent profile, automatic loopback CDP port,
and a readiness wait; no manual debugging flags or endpoint are needed. Default
launch has a visible window. Add `--headless` for an unattended browser, or use
`--headless=false` on a later start to change back. On macOS use the executable
inside the `.app` bundle; on Linux use the installed Chromium executable. The
path and launch mode are saved, so later commands need only the session ID.
The profile is separate from personal browser data; log in there when needed.
The launcher uses `--remote-debugging-port=0`; this and headless mode can affect
automation detection. Managed launch provides no bot-detection avoidance.

To attach to an external browser instead, use `--endpoint URL` and prepare the
browser as shown in the README's manual-launch example. Never combine `--chrome`
and `--endpoint`. A persistent built/installed executable is the supported background-process entry point.
`start` or `session ensure` explicitly gets or creates the selected session;
ordinary task/page/event commands also do this automatically.
`serve --endpoint URL` runs in the foreground instead. `stop`, Ctrl+C in a
foreground session, and SIGTERM use bounded cleanup. Managed Chrome is closed
after SDK cleanup, with a process-kill fallback for only the launched child;
external `--endpoint` browsers remain running. `stop` waits for browser cleanup,
the local listener, and discovery-file retirement. Retained/released/manually
opened tabs in a managed profile also close when its browser stops; the profile
itself is retained. On cancellation during startup, a client may spend up to
seven additional seconds allowing the child to clean up its owned browser.

## Session selection and configuration

Global flags precede the command, and command flags precede positional arguments:

```powershell
.\cdp-q-driver.exe --session work --endpoint http://127.0.0.1:9222 --timeout 20s session ensure
.\cdp-q-driver.exe --session work page new --name main https://example.com
.\cdp-q-driver.exe --session other --endpoint http://127.0.0.1:9222 page new --name main https://example.org
.\cdp-q-driver.exe --session work page list
.\cdp-q-driver.exe --session other page list
.\cdp-q-driver.exe --session work stop
.\cdp-q-driver.exe --session other stop
```

IDs use 1..64 lowercase ASCII letters/digits, `-`, or `_`, beginning with a
letter/digit. Path separators, traversal, and reserved Windows device names are
rejected. Aliases and tasks are local to the session: both sessions above can
have a page named `main`, and stopping one cleans up only its SDK resources.
Sessions connected to the same external default browser context share cookies/site
storage. `--chrome` sessions each have a dedicated persistent Chrome profile.

The default application data root is:

| OS | Root |
| --- | --- |
| Windows | `%LOCALAPPDATA%\cdp-q-driver` (home `AppData\Local` fallback) |
| macOS | `~/Library/Application Support/cdp-q-driver` |
| Linux and other Unix | `$XDG_DATA_HOME/cdp-q-driver`, or `~/.local/share/cdp-q-driver` when unset/relative |

`--data-dir DIRECTORY` overrides that application root. Each session uses its own
`sessions/ID/` subdirectory:

- `session.json`: saved Chrome path/headless mode or endpoint, SDK timeouts, creation/update times, last process
  generation, last-observed tasks/pages, and the latest cleanup report.
- `runtime.json`: the live loopback address, protocol version, process ID,
  generation, and a fresh random authentication token.
- `qdriver-*.log`: startup/shutdown diagnostics for background processes.
- `chrome-profile/`: persistent managed Chrome data, reused after stop/start.
- `chrome-*.log`: managed Chrome diagnostics.
- `browser.lock`: exclusive managed-profile launch marker with control/browser PIDs.

Metadata is written through a temporary file and atomic replacement before a
registry-changing RPC is acknowledged. A storage failure is reported explicitly
and does not undo already-created browser resources. Public `session info` and
`session list` redact endpoint user information and query parameters. The saved
endpoint itself can contain credentials, so keep the application directory
private. Files use mode `0600` inside a mode `0700` directory; existing
directory permissions and Windows ACLs follow the user's filesystem settings.
Runtime tokens are not printed in command results or placed in URLs.
The HTTP listener binds only to `127.0.0.1` on an ephemeral port, authenticates
every call, and rejects browser-origin and unexpected-Host requests.

First use automatically discovers Chrome, or accepts `--chrome PATH` or
`--endpoint URL`, either globally or on `start`/`serve`. A successful session saves the selected launch/connection settings
for future starts. A live process is
reused; a missing runtime is created from the saved settings. There is no guessed
default CDP address. Chrome path/mode, endpoint, or SDK-timeout changes are rejected while
that session is running; stop it first to reconfigure it. Concurrent first-use
clients join the same exclusively reserved runtime, then dispatch each original
command once. Readiness/status probes may be repeated; browser mutations are
never automatically replayed.

`start`/`session ensure` return after an authenticated readiness probe succeeds,
with `created` indicating whether this call started the process. They report
the selected session, process ID, instance, runtime-file path, and a startup-log
path when created by this call. The process leaves
stdin at EOF and starts a detached process (a hidden process on Windows).
`status`, `stop`, `session info`, and `session list` do not auto-start sessions.
`status` describes the local session process; it does not guarantee its browser
is still connected. Browser loss is reported by SDK commands. `session info`
also works after stop and returns `lastObservedTasks`, `lastObservedPages`, and
`lastCleanup`; this cached inventory is not proof that a browser target is alive.

`--timeout` defaults to 30 seconds and applies to the entire client operation,
including browser startup, communication, and waits. Set it separately on any invocation; no
server restart is required. `start`/`serve` also accept `--command-timeout`
(default 15 seconds, maximum 24 hours) and `--cleanup-timeout` (default 5 seconds,
maximum 1 minute). The SDK timeout additionally bounds each browser command;
event/download waits run outside that command gate. Chrome launch settings, the browser endpoint,
discovery path, authentication capability, and SDK options are fixed for the
control process's lifetime; changing them requires stopping and restarting it.
Saved launch/endpoint/timeouts are reused unless explicitly overridden for a new runtime.

Stopping removes `runtime.json` and keeps `session.json`. Restart preserves the
session ID/configuration but creates a fresh generation: live tasks, aliases,
refs, event queues, and handles are not reconstructed from the last-observed
inventory. Owned pages closed by stop are not reopened. Abrupt termination can
leave browser tabs and a stale runtime file. Inspect its recorded process and
confirm it has exited before removing that exact stale file; automatic ensure
does not replace an unresponsive generation whose lifetime is uncertain.
Use `page list --all` and explicitly adopt any retained tab after restarting;
previous creation ownership is not reconstructed. An abrupt crash in managed
mode can leave Chrome and `browser.lock` running/present. Do not delete the lock
until both recorded processes are confirmed exited; a stale runtime/lock does
not authorize killing an arbitrary PID. Normal stop removes both runtime and
browser lock, preserving the profile and saved configuration.

The previous `--state FILE` mode remains available for external endpoints and explicit runtime-file
selection and manual start. It does not create named-session metadata or
auto-start missing processes, cannot be combined with `--session`/`--data-dir`,
and its `start` still rejects an existing runtime. Previously created cache files
can be addressed explicitly with `--state`; they are not silently migrated.

## Tasks, pages, and refs

Each session starts with a task named `default`. Additional tasks have explicit
names. Page names are explicit with `--name` or generated as `page1`, `page2`, etc.
Commands accept a managed name or its browser target ID; there is no implicit
active-page selection shared between clients.

```powershell
.\cdp-q-driver.exe task new research
.\cdp-q-driver.exe page new --task research --name main https://example.com
.\cdp-q-driver.exe page list
.\cdp-q-driver.exe page frames main
.\cdp-q-driver.exe page goto main https://example.org
.\cdp-q-driver.exe task finish --keep main research
```

`page list` includes only managed tabs in the selected named session; use
`page list --all` to include external/unmanaged browser targets for explicit
adoption. Managed entries include their `session` ID. Legacy `--state` mode keeps
the earlier all-browser listing default. `page adopt --name borrowed TARGET_ID`
borrows one; `page close borrowed` is rejected. `page release borrowed` returns it.
`task finish` closes owned pages and releases borrowed ones. `--keep` is a
comma-separated list of owned page names to leave open and release. Finishing
`default` removes it too; create it again with `task new default` if needed.
`stop` applies the same cleanup to all remaining tasks. Kept/released tabs and
an external browser remain open. With managed launch, stopping the session
closes its entire dedicated browser; task-level `--keep` lasts until that stop.

Snapshots return `content`, `nodes`, `scope`, `documentId`, `frames`, `truncated`,
and any `unsupported` features. CLI refs are opaque tokens such as
`@a1b2c3d4e5f6.p1.12`, bound to the control-process generation, page, and exact SDK
ref. Copy an actual `nodes[].ref`; guessed SDK labels such as `@1` are rejected.
Wrong-page and previous-generation tokens never select a similar-looking node.
The CLI retains at most 10,000 refs per page, preserving currently delivered refs.
Full-page snapshots replace its active set; partial snapshots retain omitted refs
within that capacity. SDK document/session invalidation still applies.

For a fixture containing a textbox named `Name` and a `Save` button:

```powershell
$s = .\cdp-q-driver.exe page snapshot --scope full_page main | ConvertFrom-Json
$name = ($s.result.nodes | Where-Object { $_.role -eq 'textbox' -and $_.name -eq 'Name' } | Select-Object -First 1).ref
$save = ($s.result.nodes | Where-Object { $_.role -eq 'button' -and $_.name -eq 'Save' } | Select-Object -First 1).ref
.\cdp-q-driver.exe page fill --ref $name main 'Ada'
.\cdp-q-driver.exe page click --ref $save main
```

Alternatively use exactly one of `--css`, `--text`, or `--role` (with optional
`--name` for the accessible name). `--frame FRAME_ID` scopes a non-ref locator to
a frame from `page frames`; refs already carry their frame. SDK iframe/OOPIF,
open-shadow, ambiguity, visibility, and input-receipt rules are unchanged.
`page snapshot --scope subtree --root REF PAGE` selects an observed subtree.
`--max-nodes` accepts 1..10,000; `--max-bytes` accepts 1..1,048,576 content bytes.
Longer CLI ref labels can further truncate textual content; structured nodes
retain the SDK-delivered metadata and refs.

`page eval PAGE EXPRESSION` awaits a JavaScript Promise and invalidates the SDK's
refs. Pass `-` as the expression to read up to 1 MiB from stdin. JavaScript may
continue after a timeout. `page screenshot PAGE FILE` checks exclusive creation,
activates the named page, and captures a PNG; it can change the foreground tab.
`page wait-selector SELECTOR PAGE`, `page wait-function PAGE EXPRESSION`, and
`page wait-idle --idle 500ms PAGE` expose SDK waits. Network idle concerns only
observed requests from attached frame sessions; frame changes can invalidate it.

## Events, dialogs, and files

An armed subscription survives the RPC that created it. Prepare it before the
trigger, consume events in a subsequent invocation, and close it afterward:

```powershell
$armed = .\cdp-q-driver.exe events arm --kind popup main | ConvertFrom-Json
.\cdp-q-driver.exe page click --css '#open' main
$event = .\cdp-q-driver.exe events next $armed.result.subscription | ConvertFrom-Json
.\cdp-q-driver.exe popup adopt --name child $event.result.handle
.\cdp-q-driver.exe events close $armed.result.subscription
```

Kinds are `popup`, `dialog`, `fileChooser`, `download`, `request`,
`requestFinished`, and `requestFailed`. An expected popup is task-owned when its
managed opener is verified, even if the event is never consumed. Popup arrival
does not guarantee document readiness. Use `page wait-selector` afterward.

`events next` returns at most one queued event. Separate subscriptions observe
independently; concurrent Next calls on one subscription share its queue. A Next
timeout ends that wait, not the subscription. SDK queue overflow is an explicit
error. Waiting for an event or download does not block commands that trigger or
handle it. The local process permits 64 in-flight non-control RPCs; `status` and
`stop` remain available while waits occupy those slots.

Dialog, chooser, popup, and download events include an opaque `handle`. For
example, arm a `dialog` event, click its button, then call
`dialog accept HANDLE 'Ada'` or `dialog dismiss HANDLE`. The trigger may return
`dialog_open` after dispatching input. Handle the modal without repeating the
trigger. `dialog current PAGE` recovers a current modal's handle even if no
subscription was armed. Accepted/dismissed handles are retired.

For file inputs, use `page upload --css '#file' PAGE ABSOLUTE_FILE...`, or omit
files to clear the selection. For a chooser, run `chooser interception PAGE on`,
arm `fileChooser`, click its trigger, and use `chooser set HANDLE FILE...` or
`chooser cancel HANDLE`. Turn interception off when finished; page cleanup also
resets it. Chooser handles are single-use and document/session-bound.

Downloads require `download enable DIRECTORY` before arming `download`. This
explicitly changes the default browser context's download policy for other
clients too. Chrome and Go must share the path. `stop` resets the policy to
`default`, not an unknown previous policy. `download wait HANDLE` returns the
GUID-named path on completion; `download save HANDLE DESTINATION` copies it
exclusively. Existing destinations are never overwritten. `download info HANDLE`
reports progress, and `download cancel HANDLE` cancels an active transfer.
Page/task cleanup cancels active managed downloads; completed files remain.

The CLI keeps at most 1,024 event subscriptions overall (the SDK also limits each
page to 128), and 1,024 typed handles. `events close` frees a subscription;
`handle close` forgets an observed handle without relinquishing task-owned
resources. Page release/task cleanup retires associated entries. Download waits
and saves leave their handle available for inspection or explicit forgetting.

## Output and errors

Help is text. Successful operations write one JSON envelope to stdout:

```json
{"ok":true,"result":{"name":"main","id":"TARGET_ID","task":"default","owned":true,"managed":true}}
```

Failures write a JSON envelope to stderr with a nonzero exit code:

```json
{"ok":false,"error":{"code":"stale_ref","message":"ref is stale; take a new snapshot","mayHaveEffects":false}}
```

Exit codes are 0 for success, 1 for operation/transport failures, and 2 for usage
errors. Top-level CLI fields use camel case. SDK payloads such as frame metadata,
action receipts, dialog info, download info, and cleanup reports retain their
exported Go field names (`ID`, `InputCompleted`, `Closed`, etc.). A failed call
can include a partial result such as its action receipt or newly created page.
`mayHaveEffects`, and optional SDK `operation`/`pageId`, preserve dispatch
uncertainty. Transport loss after a mutating RPC is conservatively uncertain;
inspect current state before retrying. Event-consumption acknowledgements can
also be lost: an event is not replayed after an RPC failure.

Requests are limited to 2 MiB and encoded responses to 16 MiB. Unsupported
commands, invalid selectors, stale handles, missing tasks, authentication,
timeouts, overflow, and browser loss fail explicitly. This local protocol is
versioned internal CLI transport, not a public network service or a durable job
system.
