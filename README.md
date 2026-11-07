# cdp-q-driver

A Go SDK and stateful CLI for task-owned Chromium tabs and snapshot-driven input over the Chrome DevTools Protocol (CDP).

A stateful CLI is available at `cmd/cdp-q-driver`. Select a named session with
`--session ID`; missing sessions start automatically and reuse their saved CDP
configuration. It finds installed Chrome/Chromium automatically when no browser
settings are supplied. Pass `--chrome PATH` to choose an executable, or
`--endpoint URL` to attach to an existing debugging browser. Session metadata
and managed Chrome profiles live in the OS application data directory.
Each session keeps tasks, page names, refs, and event subscriptions alive across
terminal invocations while preserving
the SDK's existing import path. See the [CLI guide](docs/cli.md) for all commands,
JSON outcomes, session lifetime, and file/event workflows.

The first four milestones implement tab lifecycle management, frame-aware observation/input, and event/file workflows. Launch a dedicated browser or connect to an explicit endpoint, capture semantic snapshots across iframe/OOPIF and open shadow-root boundaries, select exact nodes by opaque refs or CSS/text/role, and verify input results. Prepare subscriptions before triggering popups, dialogs, file choosers, downloads, or network requests. SDK cleanup preserves borrowed tabs and the external browser; stopping a CLI-managed Chrome session also terminates its dedicated browser.

Closed shadow roots and non-axis-aligned frame transforms are not supported.

## Requirements

- Go 1.27.3 or newer, matching `go.mod`.
- An installed Chrome/Chromium browser (automatically found on PATH or in standard
  OS installation locations), an explicit `--chrome PATH`, or an existing
  debugging endpoint supplied with `--endpoint URL`.

The SDK's `Attach` does not start, install, or discover a user's browser; the CLI's
`--chrome` mode manages launch separately. `Attach` accepts an HTTP(S) discovery URL, such as `http://127.0.0.1:9222`, or a full browser WebSocket URL ending in `/devtools/browser/<id>`. Page WebSocket URLs are rejected.

## Install or build

Add the SDK to an existing Go project:

```sh
go get github.com/snowmerak/cdp-q-driver
```

Install the CLI from the current `main` revision:

```sh
go install github.com/snowmerak/cdp-q-driver/cmd/cdp-q-driver@main
```

The executable is placed in `GOBIN`, or Go's default binary directory when
`GOBIN` is unset. Put that directory on `PATH` to invoke `cdp-q-driver` directly.
To build this checkout into the repository root on Windows:

```powershell
go build -o cdp-q-driver.exe ./cmd/cdp-q-driver
.\cdp-q-driver.exe help
```

On Linux/macOS, use `go build -o cdp-q-driver ./cmd/cdp-q-driver` and run
`./cdp-q-driver`. From a checkout, `go install ./cmd/cdp-q-driver` also installs
the local CLI. The module-root package remains importable as the Go SDK.

## Agent skill

The repository ships one portable Agent Skill at
[`skills/cdp-q-driver`](skills/cdp-q-driver/SKILL.md). It teaches agents to use
named sessions, snapshot refs, native input, event/file workflows, and scoped
cleanup with this CLI. Its supporting resources are included in the skill folder
so an installed copy does not depend on this repository checkout.

With a GitHub CLI version that provides `gh skill`, install it for Codex:

```sh
gh skill install snowmerak/cdp-q-driver cdp-q-driver --agent codex --scope user
```

Use `--scope project` for installation in the current project's skills directory,
or choose another host with `--agent`. Invoke it as `$cdp-q-driver` in a supporting
agent. The skill requires the CLI executable and an installed browser or an
explicit browser CDP endpoint. The CLI discovers Chrome automatically and prepares
a dedicated browser; `--chrome PATH` overrides discovery. Installing the skill alone does
not install Go, the CLI, or Chrome.

To inspect before installation, use
`gh skill preview snowmerak/cdp-q-driver cdp-q-driver`. For a reproducible install,
add `--pin COMMIT_SHA` with the desired commit. See the
[GitHub CLI skill-install documentation](https://cli.github.com/manual/gh_skill_install)
for version selection and supported hosts. From a checkout, validate the package
without publishing a release with `gh skill publish --dry-run`.

## CLI sessions

Select a session and open a page. The CLI finds Chrome, creates a dedicated
persistent profile, starts the browser, selects a loopback CDP port, waits for
readiness, and opens the named page:

```powershell
.\cdp-q-driver.exe --session work page new --name main https://example.com
.\cdp-q-driver.exe --session work page list
.\cdp-q-driver.exe --session work page snapshot main
.\cdp-q-driver.exe --session work session info
.\cdp-q-driver.exe session list
.\cdp-q-driver.exe --session work stop
```

The default browser has a visible window; add `--headless` for unattended use.
Explicit `--chrome PATH`/`--endpoint URL` settings take precedence, followed by
saved session settings, `QDRIVER_CHROME`, PATH, and standard OS installation
locations. Discovery checks Chrome/Chromium, not personal profiles or running
debugging ports. If none is found, the error asks for an executable or endpoint.
A selected but invalid explicit/saved/environment path reports an error instead
of silently choosing another browser.

To choose a particular Windows installation, pass
`--chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe'` before the
command. On macOS, pass the executable inside the app bundle, for example
`/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`; on Linux, pass
the installed Chrome/Chromium executable. To attach to an already running
debugging browser instead, use `--endpoint http://127.0.0.1:9222` in place of
`--chrome PATH`. These modes are mutually exclusive.

The first task/page/event command creates the selected session when absent.
Later calls reuse its running control process, page aliases, refs, and armed
subscriptions. A successful first start saves the Chrome path/headless setting
or external endpoint, along with SDK timeouts. Automatically discovered Chrome
paths are saved too. Future starts omit these flags.
`start` and `session ensure` explicitly get
or create a session. `status`, `stop`, `session info`, and `session list` inspect
or stop sessions without creating one. The default session ID is `default`.
Global flags precede the command, and command flags precede positional arguments.

| Command | Purpose |
| --- | --- |
| `session list`, `session info` | Inspect saved sessions and the selected session's last-observed inventory. |
| `task new NAME`, `task finish NAME` | Group pages and clean up the task's owned resources. |
| `page new --name NAME URL`, `page list` | Open and list managed tabs in the selected session. |
| `page list --all`, `page adopt --name NAME TARGET_ID` | Find external tabs and explicitly borrow one. |
| `page snapshot PAGE` | Observe a page and obtain opaque CLI refs. |
| `page fill --ref REF PAGE TEXT`, `page click --ref REF PAGE` | Act on exact nodes delivered by a snapshot. |
| `events arm --kind KIND PAGE`, `events next SUBSCRIPTION` | Prepare an event before its trigger, then wait independently. |
| `stop` | Clean up the selected session and stop its control process; also close Chrome when this session launched it. |

Different sessions can each have a page called `main`. Each session's `page list`
shows its managed tabs; `--all` includes external targets. Stopping a session
cleans up its SDK resources; in `--chrome` mode it also closes its dedicated
browser, including any retained/released/manually opened tabs in that profile.
The profile remains on disk, preserving logins/site storage for later starts.
Each `--chrome` session has its own profile. Sessions attached with `--endpoint`
to the same browser context share cookies and site storage.

Session data is stored beneath `sessions/<ID>/` in the application data root:

| OS | Default application data root |
| --- | --- |
| Windows | `%LOCALAPPDATA%\cdp-q-driver` |
| macOS | `~/Library/Application Support/cdp-q-driver` |
| Linux | `$XDG_DATA_HOME/cdp-q-driver`, or `~/.local/share/cdp-q-driver` |

`session.json` keeps connection/Chrome launch settings, the last-observed Task/page grouping,
and the latest cleanup report. `runtime.json` holds the running process's local
address and authentication capability; stop removes that runtime file while
preserving the saved session. Managed Chrome also uses `chrome-profile/`,
`chrome-*.log`, and a `browser.lock` while owned by the control process.
`--data-dir DIRECTORY` overrides the application
data root. Keep these files private because connection settings may contain
credentials. See the [CLI guide](docs/cli.md) for path fallbacks, ID rules, and the
legacy explicit-file `--state FILE` mode.

Restarting a saved session creates a new generation. Previously closed tabs,
live task/page aliases, snapshot refs, and event queues are not restored from
the observed inventory. Runtime refs are bound to their session generation and
page; copying a ref between sessions or across restarts fails explicitly.
Abrupt termination can leave a stale runtime file: confirm its recorded process
has exited before removing that exact file. The CLI never automatically replays
a browser mutation after an uncertain response.
An abrupt control-process crash can leave its managed Chrome and `browser.lock`
behind. Confirm the recorded control/browser processes have exited before
removing the exact stale runtime/lock files; the CLI will not guess or kill a
process from cached metadata. The persistent profile is never deleted by stop.

Managed launch uses a nonzero debugging port, and page/frame sessions disable
Runtime event streaming after initialization to reduce unnecessary automation
signals. The default visible browser retains its native browser properties and
User-Agent. This is not an undetectable browser: headless and input-behavior
checks can still flag it. One Google search loaded results without a challenge;
this does not guarantee future CAPTCHA behavior.
See the [measured results and limits](docs/research/automation-signals.md).

## Try it

Start a dedicated test browser. For example, in PowerShell:

```powershell
$exampleProfile = Join-Path $env:TEMP ('qdriver-example-' + [guid]::NewGuid())
$exampleBrowser = Start-Process -WindowStyle Hidden -PassThru `
  -FilePath 'C:\Program Files\Google\Chrome\Application\chrome.exe' `
  -ArgumentList @(
    '--headless=new',
    '--remote-debugging-address=127.0.0.1',
    '--remote-debugging-port=9222',
    ('--user-data-dir="' + $exampleProfile + '"'),
    '--no-first-run',
    'about:blank'
  )
```

Adjust the executable path for your installation. Once the CDP endpoint is ready, run:

```sh
go run ./examples/tabs -endpoint http://127.0.0.1:9222
```

The example opens a tab, loads a self-contained HTML page, reads its title, closes the tab, and prints a JSON cleanup report. The title should be `QDriver demo`, and `Cleanup.Closed` should contain `PageID`. The browser's original tabs remain open.

Run the snapshot/ref/input example against the same browser:

```sh
go run ./examples/interact -endpoint http://127.0.0.1:9222
```

This example observes a self-contained form, finds its textbox and button refs, fills the textbox, clicks Save, and verifies the output. Its JSON `Result` should be `Go CDP works`. See the [complete interaction example](examples/interact/main.go) for error handling and cleanup.

The [frames example](examples/frames/main.go) captures an iframe's controls, fills/clicks their refs, and clicks a button inside an open shadow root:

```sh
go run ./examples/frames -endpoint http://127.0.0.1:9222
```

Its JSON `Result` should be `Go frames work`, with two entries in `Frames`. Actual cross-site OOPIF coverage lives in the integration tests described below.

The SDK leaves the example browser running. Stop only the browser process you started when finished:

```powershell
Stop-Process -Id $exampleBrowser.Id
```

The [example source](examples/tabs/main.go) shows the complete error-handling and cleanup flow. Import the library as:

```go
import qdriver "github.com/snowmerak/cdp-q-driver"
```

## API

All operations accept a `context.Context`. `Options.CommandTimeout` defaults to 15 seconds and includes time waiting for another operation. `Options.CleanupTimeout` defaults to 5 seconds for the entire cleanup.

| Operation | Behavior |
| --- | --- |
| `Attach(ctx, endpoint, options)` | Connect without selecting or creating a tab. Its context controls attachment only. |
| `Browser.Pages(ctx)` | List page IDs, URLs, and titles without adopting them. |
| `Browser.NewTask(ctx, name)` | Create a logical task sharing the browser's default context. |
| `Task.NewPage(ctx)` | Open and attach to an owned `about:blank` tab. |
| `Task.Adopt(ctx, targetID)` | Borrow an existing page; reject duplicate ownership on the same connection. |
| `Page.Goto`, `Title`, `URL` | Navigate with a load wait, or read the current document. |
| `Page.Evaluate` | Evaluate a top-document expression, await a Promise, and return JSON. Exceptions are errors. |
| `Page.Screenshot` | Return a viewport screenshot as PNG bytes. |
| `Page.Activate` | Bring the managed tab to the foreground. |
| `Page.Frames(ctx)` | List reachable frame IDs, parent IDs, names, URLs, and `OutOfProcess` status. |
| `Page.Snapshot(ctx, options)` | Combine AX and DOM/layout across frames/open shadows into bounded viewport, full-page, or subtree output. |
| `Page.Click(ctx, locator)` | Activate the tab, scroll into view, check the exact hit target, and send one left click. |
| `Page.Fill(ctx, locator, text)` | Replace a supported text control's contents and verify the retained text. |
| `Page.Press(ctx, locator, key)` | Focus a control and send a key or modifier chord, such as Enter or Control+A. |
| `Page.WaitForSelector(ctx, locator)` | Wait for a single visible element; ambiguity and stale refs fail immediately. |
| `Page.WaitForFunction(ctx, expression)` | Poll a synchronous, truthy page expression. Like Evaluate, this invalidates refs. |
| `Page.Subscribe(ctx, kinds...)` | Arm independent, bounded event streams; call `Next(ctx)` and `Close()`. |
| `Page.ExpectPopup(ctx)` | Arm a single popup ownership claim, verified against its managed opener. |
| `Page.ExpectDialog(ctx)`, `CurrentDialog()` | Observe one modal and explicitly accept or dismiss its specific handle. |
| `Page.SetInputFiles(ctx, locator, paths...)` | Set absolute file paths on a file input, including hidden controls; empty paths clear it. |
| `Page.SetFileChooserInterception(ctx, enabled)` | Explicitly intercept native file choosers across attached page/frame sessions. |
| `Browser.EnableDownloads(ctx, directory)` | Explicitly configure the default context's download policy and GUID-named output files. |
| `Page.ExpectDownload(ctx)` | Subscribe to download starts; each handle supports `Wait`, `Cancel`, and exclusive `SaveAs`. |
| `Page.ExpectNetworkIdle(ctx, idleFor)`, `WaitForNetworkIdle(ctx, idleFor)` | Wait outside the operation gate for a quiet interval in observed page/frame requests. |
| `Page.Close` | Close an owned tab and confirm its disappearance. Borrowed tabs return `ErrNotOwned`. |
| `Task.Release(ctx, page)` | Relinquish a page without closing it; invalidate its old handle. |
| `Task.Finish(ctx, keep...)` | Close owned tabs, release borrowed tabs, and retain explicitly named owned tabs. |
| `Browser.Close(ctx)` | Interrupt calls, clean up tasks, and disconnect without sending `Browser.close`. |

If tab creation succeeds but session setup fails, `NewPage` can return a non-nil page together with an error. The task still tracks it for cleanup. `Adopt` has the same behavior for a failed borrowed session setup. A failed session can be closed (if owned) or released.

## Snapshots, refs, and input

`SnapshotOptions.Scope` defaults to `Viewport`; `FullPage` includes off-screen rendered nodes without scrolling. `Subtree` requires a current `Root` ref and can select an iframe boundary, an open shadow host, or another ref-bearing element. Capturing a subtree does not scope subsequent actions; use the returned refs or an explicit frame locator. `MaxNodes` defaults to 1,000 and `MaxBytes` to 64 KiB of `Content`. Limits apply to emitted output, not the underlying AX/DOM collection. `Truncated` reports an output/ref-capacity limit. `Unsupported` identifies omitted closed-shadow, unsupported frame geometry, canvas, pseudo-element, or unavailable layout content. Password field values are omitted.

Snapshots provide `Content`, structured `Nodes`, and opaque refs on interactive elements and iframe/open-shadow boundaries. `SnapshotNode.FrameID` and `Snapshot.Frames` retain the provenance of delivered content. Select a node's ref or look up its displayed label using `snapshot.Ref(label)`, then pass `qdriver.ByRef(ref)` to an action.

`qdriver.CSS(selector)` searches the document and nested open shadow roots. `qdriver.Text(text)` matches exact whitespace-normalized text on the smallest matching DOM elements. `qdriver.Role(role, name)` uses Chromium's accessibility role and computed name, with exact matching. Without a frame scope, any top-document matches take precedence; otherwise exactly one match across descendant frames is required. CSS/text count hidden matches too; input then checks actionability. Ambiguity returns `ErrAmbiguous`, and invalid CSS returns `ErrInvalidLocator`.

Restrict any locator to a current frame using `qdriver.CSS("#name").InFrame(frame.ID)` or `qdriver.Role("button", "Save").InFrame(frame.ID)`. Frame IDs come from `Page.Frames` or snapshot metadata. An empty or removed explicit frame ID never falls back to the root. Refs already carry their frame/document identity.

Refs are tied to the exact page, frame, renderer session, document, and backend node. Unchanged nodes retain their refs across snapshots and high-level actions. Viewport/subtree captures preserve omitted refs; full-page captures replace the active set with delivered refs. `Options.MaxRefs` defaults to 10,000 per page; when retaining partial observations exceeds that limit, omitted refs are evicted oldest-first. Refs delivered in the current snapshot remain usable. Frame navigation, removal, or renderer/session replacement invalidates affected refs while preserving unrelated frames. `Evaluate` and `WaitForFunction` conservatively invalidate all refs. A ref from another page returns `ErrWrongPage`, and stale refs never fall back to a similar-looking element.

Snapshot viewport filtering intersects each frame's viewport through its ancestors; it is not a complete CSS clipping/hit-test guarantee or an atomic snapshot of all same-document mutations. Actions independently recheck visibility, enabled state, document identity, deep shadow focus, and pointer interception, including ancestor iframe overlays. Coordinates include frame borders and positive axis-aligned scaling. A same-process iframe's hover-induced scroll correction may reposition the pointer once before the first press; a click itself is never replayed. Rotation, skew, perspective, and mirrored frame transforms return `ErrUnsupported`. Native input activates the managed tab, so it can change which tab is visible.

`Fill` supports text/search/email/tel/url/password inputs, textareas, and plain contenteditable hosts, including controls in supported frames/open shadow roots. It selects existing contents, clears them with Backspace, and types characters using native keydown/keyup sequences with bounded timing jitter. Printable US-layout characters include physical key codes and modifier transitions. Unicode and literal newline/tab characters use individual native text insertions between `Unidentified` key events; this does not emulate IME composition. An empty replacement clears the control. Each character rechecks document identity, focus, editability, and interception. Application formatting can cause `ErrFillRejected` after the complete edit; interrupted typing can leave partial text. Input handlers now observe per-character events. Number/date inputs, rich editors, closed shadow roots, and other widgets remain outside the supported input set.

`Press` supports Enter, Tab, Escape, Backspace, Delete, arrows, Home/End, PageUp/PageDown, printable characters, and physical Alt/Control/Meta/Shift down/up sequences. Cancellation releases attempted keys without replaying their presses. It does not provide IME composition.

Pointer movement uses an eased cubic Bezier curve before the existing
post-hover/hit-target checks. Distance and visible target size adjust duration
and point count; the endpoint remains the exact inspected hit point, without
overshoot or randomized landing. It starts from the driver's last acknowledged
position in the same renderer session, or that renderer's viewport origin when
unknown. It does not read the OS pointer or track another client's movement.
Defaults use at least 12 points (up to 120), a 180 ms base movement duration
scaled by 0.6..3.5, a 40 ms mouse-button hold, a 35 ms key hold, and a 65 ms
base key-onset interval, with up to ±15 ms duration jitter. QWERTY key distance,
row, hand, finger, repetition, and entering Shift adjust onset intervals.
Eligible opposite-hand transitions with matching modifiers have a 15% chance
of overlapping; at most two character keys are held. Repeated keys, modifier
changes, and Unicode fallback flush held keys first. Cancellation attempts to
release held character keys and Shift without replaying input. CDP work and
target checks add time and can prevent a planned overlap. These are geometric
and timing heuristics, not a model fitted to recorded human behavior, and do
not guarantee a human verdict. Korean text still uses the Unicode fallback;
physical dubeolsik composition is not implemented.

`Options.InputTiming` customizes these values. Nil uses the defaults;
`&qdriver.InputTiming{}` disables interpolation/delays while retaining character
key events. Explicit structs use their supplied values, including zero; each
base duration must be 0..2 seconds, and `MouseSteps` 0..120 (0 means one point
when curves are disabled). `MouseCurve` enables adaptive curves;
`KeyTransitions` enables QWERTY transition costs, and `KeyOverlap` is a 0..1
probability. With transitions or overlap enabled, `KeyInterval` is an onset
interval; otherwise it is a gap after key release. Explicit structs leave
these features disabled unless requested.
The CLI defaults to `--input-mode paced`; `--input-mode fast` selects the zero
timing settings. The mode is saved per session and requires stopping/restarting
to change. Increase both the CLI `--timeout` and SDK `--command-timeout` for long
text, or choose fast mode. Inspect partial state after a timeout before any
follow-up edit.

Input returns an `ActionReceipt`: `InputStarted` means a CDP input command may have been sent, including a pointer move; `InputCompleted` means the entire sequence was acknowledged. Neither means the application completed its work. Tab activation, scrolling, and focus can have effects even before input starts. On cancellation during a pressed gesture, one cleanup release may still complete that gesture under the cleanup budget; the press itself is never replayed. Use explicit page assertions/waits to verify the business result.

Frame discovery merges renderer frame trees with browser-reported `parentFrameId` relationships. Only reachable OOPIF targets receive extra sessions, using named isolated worlds for DOM helpers without universal access. The SDK uses explicit attach rather than pausing targets/workers with auto-attach. `Options.MaxFrames` defaults to 128; exceeding it returns `ErrFrameLimit`. Frame collection can retry a transient renderer/context change before input, and selector waits can tolerate an in-progress frame transition within their deadline. Dispatched input is never retried. Older browser versions without the parent-frame metadata have not been verified.

## Events, dialogs, and files

Use **prepare → trigger → wait → close**. `Subscribe` returns only after observation is ready, and subscribers do not consume each other's events or the internal network tracker. Pass event kinds to filter noise: `PopupOpened`, `DialogOpened`, `FileChooserOpened`, `DownloadStarted`, `RequestStarted`, `RequestFinished`, or `RequestFailed`. An empty kind list observes all of them. `Event` carries the page/frame identity and the corresponding typed payload. Request IDs are scoped to `Request.SessionID`; headers and bodies are omitted.

For example, with an existing managed `p` and `ctx`:

```go
popups, err := p.ExpectPopup(ctx)
if err != nil {
    return err
}
defer popups.Close()
if _, err := p.Click(ctx, qdriver.CSS("#open")); err != nil {
    return err
}
event, err := popups.Next(ctx)
if err != nil {
    return err
}
popup, err := event.Popup.Page(ctx)
if err != nil {
    return err
}
// popup is owned by the opener's task and is closed by Finish.
```

`ExpectPopup` claims the first subsequent opener-verified popup for cleanup, even if its event is never consumed or its page is never attached. Closing the subscription stops future claims; it does not relinquish an already claimed tab. An ordinary popup `Subscribe` observes without claiming ownership; `event.Popup.Page(ctx)` then borrows the tab. Popups without a verified managed opener are not automatically claimed. Popup notification does not imply document load; use a selector or explicit page condition afterward.

Subscriptions live until their setup context is canceled, `Close` is called, or the page/connection ends. `Next` additionally accepts a per-wait deadline and does not hold the operation gate or inherit `CommandTimeout`. Canceling a `Next` call leaves its subscription active. `Options.EventBuffer` defaults to 64 queued events per subscription and `MaxSubscriptions` to 128 per page. Queue overflow terminates that stream with `ErrEventOverflow`; it never looks like a complete event history. Closing a stream discards its queued events.

A JavaScript dialog can stall renderer commands. Its opening interrupts the active SDK renderer operation with `ErrDialogOpen`, releasing the gate so `event.Dialog.Accept(ctx, promptText)` or `Dismiss(ctx)` can handle it. Inspect `ActionReceipt`/`OperationError`: the triggering input or script may already have run. A successful triggering call is also possible if Chromium acknowledged it before the modal opened. Do not retry the trigger. Renderer commands reject an already-open modal; `CurrentDialog()` provides its handle even without a prepared subscription. Old handles return `ErrDialogStale`. Deferred input/object cleanup resumes after handling the modal. Browser policies can suppress dialogs; the SDK does not bypass them. Returning a borrowed page dismisses any observed modal before detaching.

`SetInputFiles` validates absolute, existing regular files and an exact file input; multiple files require `multiple`. It supports hidden inputs and supported frame/open-shadow locators. Files must exist at the same paths for Go and Chrome: remote file transfer is not implemented. Empty paths clear the selection through the DOM value setter and dispatch synthetic `input`/`change` events when it changed, because Chromium may acknowledge an empty CDP file list without clearing it. Other selections use `DOM.setFileInputFiles`.

For a native chooser workflow, explicitly enable `SetFileChooserInterception(ctx, true)`, subscribe to `FileChooserOpened`, then click its trigger. `event.FileChooser.SetFiles(ctx, paths...)` addresses the exact originating input; `Cancel(ctx)` supplies no files and clears its selection. Chooser handles are single-use and become stale on navigation or session replacement. Interception remains enabled until explicitly disabled or the page is returned/closed. Newly discovered OOPIF sessions inherit it when attached; an undiscovered frame cannot be intercepted retroactively. Release/Finish reset interception on borrowed sessions before detaching.

Downloads require `EnableDownloads` with an absolute directory before `ExpectDownload`. This **changes the default browser context's policy for all clients**, using `allowAndName` and enabled download events. Use a dedicated debugging browser. `Browser.Close` resets the policy to `default`; CDP cannot read back and restore another client's previous policy. Suggested filenames remain metadata; the on-disk name is the browser GUID. `event.Download.Wait(ctx)` distinguishes completion from `ErrDownloadCanceled` (cancellation or transfer failure). `SaveAs(ctx, absoluteDestination)` waits for completion and copies into a new file without overwriting an existing destination; its parent must exist. Go and Chrome must share the download directory. Active downloads originating in managed frames are canceled when that page is returned or its task finishes, even without a subscriber; completed files remain for the caller to manage.

Network idle is optional. Prepare `idle, err := p.ExpectNetworkIdle(ctx, idleFor)` before an action, then `idle.Wait(ctx)` afterward; zero duration means 500 ms. `WaitForNetworkIdle` prepares and waits immediately. The tracker observes the page and attached OOPIF sessions independently of public event queues, counts redirected requests once per session/request ID, and removes completed/failed requests. WebSocket and EventSource requests are excluded. It cannot reconstruct requests started before attachment and is not an application-readiness guarantee. Frame attachment/removal or OOPIF session changes invalidate an existing wait with `ErrNetworkTrackingChanged`; discover the new frames and prepare again. Tracking capacity is 4,096 in-flight requests; overflow makes subsequent idle waits fail rather than report a false quiet period.

Run the complete [events example](examples/events/main.go) against a dedicated browser:

```sh
go run ./examples/events -endpoint http://127.0.0.1:9222
```

It verifies an owned popup, prompt handling, a hidden file input, a completed download, network idle, and cleanup using local fixtures and temporary files.

## Lifecycle and errors

- The SDK preserves external tabs. Adopting a tab never changes it into an owned tab, and a kept or released page is no longer owned by the old handle.
- `Finish` and `Close` use a fresh cleanup timeout even when their input context has been canceled. Cleanup reports distinguish confirmed `Closed`, `Released`, `Kept`, and unconfirmed `Remaining` target IDs.
- A partial `Finish` can be retried, but that task no longer accepts new work. `Browser.Close` is final and repeatable; it returns the same cleanup report.
- A timeout does not undo a navigation or JavaScript side effect. An `OperationError` preserves the cause for `errors.Is` and conservatively reports `MayHaveEffects`. Mutating operations are never automatically retried.
- A timed-out create request may have created a tab before its ID was received. Such an unknown tab cannot be included in automatic cleanup; inspect the browser's page list rather than blindly retrying the request.
- Commands are serialized per browser connection. Event delivery and waits run independently of that command gate. Separate logical tasks share cookies and site storage; they are not security sandboxes.
- A lost connection invalidates the connection. There is no automatic reconnect or command replay.

## Tests

```sh
go mod verify
go build ./...
go vet ./...
go test -v -count=1 -timeout=180s ./...
```

Integration tests start their own headless Chrome/Chromium/Edge process, temporary profile, and ephemeral CDP port. They use local HTTP fixtures and clean up only those resources. They never connect to a user's running browser.

Set `QDRIVER_CHROME` to select a browser executable. Without an installed browser, integration tests skip; set `QDRIVER_INTEGRATION=1` to make a missing browser a failure. For example:

```powershell
$env:QDRIVER_INTEGRATION = '1'
$env:QDRIVER_CHROME = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
go test -v -count=1 -timeout=180s ./...
```

Coverage includes tab lifecycle and preservation, cancellation and connection loss, all four built examples, Unicode/empty text editing, ref identity and stale/wrong-page rejection, ambiguous/hidden/disabled/covered targets, hover overlays, formatting rejection, viewport/full-page budgets, retained-ref capacity, asynchronous waits, and cancellation after a mouse press without replay.

M3 tests use three local `.test` sites mapped to loopback with `--site-per-process`. They verify actual `iframe` targets, sibling/nested OOPIFs, same-process frames, border/scale/scroll coordinates, native clicks and typing, child-frame ambiguity, frame navigation/removal/renderer swaps, iframe/shadow subtrees, nested open shadows and slots, closed-root exclusion (including nested iframes), borrowed-page/OOPIF preservation, and frame limits. A focused unit test forces renderer-local backend ID collisions; real-browser tests verify the resulting targeting behavior.

Selector-based waits tolerate transient frame-lifecycle errors between discovery
and AX/DOM inspection; a real OOPIF replacement-churn fixture verifies eventual
visibility. Exact ref failures remain errors, and dispatched input is never replayed.

The implementation has been exercised on Windows/ARM64 with Go 1.27.3 and Chrome 156.0.8078.80 (CDP 1.3). Other platforms have browser discovery helpers but have not been run here. Run `go test -race ./...` on a platform with Go race-detector support.

M4 tests cover independent subscribers, unconsumed popup ownership, modal interruption and chained stale dialog handles, borrowed-page modal/interception cleanup, hidden/multiple/shadow file inputs, chooser selection/cancel/navigation, completed/canceled downloads, exclusive SaveAs, active-transfer cleanup, public-stream overflow alongside network idle, subscription limits, failed requests, actual transport loss, and page/browser closure. Real OOPIF tests verify popup, chooser, upload, download, and request provenance, plus conservative idle failure on frame removal. Same-origin iframe dialogs are exercised without bypassing Chromium's cross-origin modal policy.

CLI tests compile the actual command, start its background and foreground control
processes, and drive separate invocations against test-owned Chrome. They cover
snapshot refs across calls, wrong-page/restart rejection, iframe locators,
exclusive screenshots, task keep/borrow semantics, popup/dialog/chooser/download
workflows, concurrent event waits, authentication, per-wait cancellation,
uncertain evaluation outcomes without replay, startup failure, and bounded
process/discovery-file cleanup.

Named-session tests cover first-use creation, saved configuration after stop,
independent page/task grouping on a shared browser, scoped/default vs all-tab
listing, cross-session refs, endpoint/timeout conflicts, concurrent creation,
app-data metadata, validation without process creation, and portable OS path rules.

Managed-browser tests exercise Chrome-path-only startup, cross-invocation
snapshot/ref input, concurrent first use, saved path/mode reuse, persistent site
storage, mode conflicts, failed-start cleanup, and owned-browser shutdown.
Discovery tests cover ordered PATH/OS candidates, environment overrides,
missing-browser errors, saved-settings precedence, and actual CLI startup
without a Chrome path or CDP endpoint.
Pacing tests inspect trusted mouse/key events, interpolation timing, curve
bounds and target-dependent duration, QWERTY transitions, native key rollover,
shifted punctuation, Unicode/newline/tab preservation, focus changes,
cancellation releasing overlapping keys/Shift without replay, and persisted
CLI input modes.

## Design

- [Project blueprint](docs/blueprint.md) — scope, architecture, milestones, and acceptance criteria (Korean).
- [Ego Lite feasibility research](docs/research/ego-lite-cdp-feasibility.ko.md) — source analysis and observed behavior (Korean).
