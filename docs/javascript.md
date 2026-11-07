# JavaScript workflows

`run` combines several existing browser commands in one async JavaScript
function body. The Node host sends commands to the same Go session process as
ordinary CLI invocations. Ref identity, native paced input, frame checks,
event handles, and mutation error semantics remain unchanged. It reduces
agent/tool invocations; it does not eliminate the underlying browser RPCs.

## Run a script

Node.js 18+ is required only for `run`. No npm package installation is needed.
Use an installed `node` on PATH, or select it with the command's `--node` flag.

```sh
cdp-q-driver --session research --timeout 60s run workflow.js https://example.com
cdp-q-driver --session research run --node /path/to/node workflow.js
```

Global options precede `run`; `--node` precedes the script path. Arguments
after the script path are passed unchanged in `session.args`. `run -` reads
the body from stdin. Scripts are function bodies rather than ES modules:
top-level `await` and `return` work, static `import`/`export` do not. `require`
is available and resolves relative modules from the script's directory
(the current directory for stdin). The working directory remains the CLI's.

Example `workflow.js`:

```js
const page = await session.newPage("links", session.args[0]);
await page.waitForFunction(() => document.querySelectorAll("a[href]").length > 0);

return await page.evaluate(() =>
  [...document.querySelectorAll("a[href]")].map(a => ({
    title: a.textContent.trim(),
    url: a.href,
  }))
);
```

To reuse a tab, use `session.page("main")` instead of `newPage`. This produces
a name-based handle; the Go service validates that the page is managed by the
selected session. An absent named session starts on the first browser command,
using the normal saved settings/discovery rules. A pure local script does not
start Chrome. `session.status()` does not start a missing session. Legacy
`--state` runs require an already-started process.

## Host API

Every browser method returns a Promise. Commands execute in submission order
through this runner; `Promise.all` does not introduce parallel browser calls.
Await each operation, especially before starting a dependent action or wait.

| API | Behavior |
| --- | --- |
| `session.name`, `session.args` | Selected session name and script arguments. |
| `session.page(name)` | Get a local handle to an existing named page; no RPC. |
| `await session.newPage(name, url = "", {task = "default"} = {})` | Create and manage a tab, returning a page handle. |
| `await session.pages()` | List managed tabs. |
| `await session.status()` | Read running session status. |
| `await session.command(...strings)` | Execute existing CLI command tokens and return its unwrapped `result`. |
| `await page.goto(url)`, `title()`, `url()`, `frames()` | Navigation and observation. |
| `await page.activate()`, `close()`, `release()` | Existing tab lifecycle operations. |
| `await page.snapshot({scope, root, maxNodes, maxBytes} = {})` | Semantic snapshot with opaque refs and truncation metadata. |
| `await page.click(locator)` | Native click with the existing action receipt. |
| `await page.fill(locator, text)`, `press(locator, key)` | Native replacement typing/key input. Korean text is supported by the Unicode fallback, without real IME composition. |
| `await page.upload(locator, absolutePaths)` | Existing file-input upload behavior. |
| `await page.evaluate(expressionOrFunction, ...jsonArgs)` | Top-document JavaScript evaluation, awaiting a returned Promise. |
| `await page.waitForFunction(expressionOrFunction, ...jsonArgs)` | Poll a synchronous truthy top-document predicate. |
| `await page.waitForSelector(locator)` | Wait for one unique visible match. Multiple matches are an error. |
| `await page.waitForIdle("500ms")` | Wait for observed network quiet; not application readiness. |
| `await page.screenshot(path)` | Save a new PNG without overwriting. |

A locator is a CSS string, or an object with exactly one of `css`, `ref`,
`text`, or `role`. Role locators may add `name`; selector-based locators may
add `frame`. Refs already carry their frame. Examples:

```js
await page.fill({ role: "textbox", name: "Search" }, "안녕하세요");
await page.click({ css: "#submit", frame: observedFrameID });
const snapshot = await page.snapshot({ scope: "full_page" });
const button = snapshot.nodes.find(n => n.role === "button" && n.name === "Save");
await page.click({ ref: button.ref });
```

`evaluate` and `waitForFunction` callbacks run inside the page, with DOM globals
but without the host's `session`, `require`, or local closures. Pass captured
values as JSON arguments:

```js
const selector = "#list a"; // Choose from observed page structure.
await page.waitForFunction(css => document.querySelectorAll(css).length >= 5, selector);
return await page.evaluate(css =>
  [...document.querySelectorAll(css)].map(a => a.href), selector);
```

Both operations invalidate refs. Take another snapshot before using refs
after either. A timed-out page expression may continue executing in Chrome;
terminating the Node host does not roll back page-side JavaScript.

## Events and other commands

`session.command` covers tasks, event subscriptions, popup/dialog handling,
file choosers, and downloads without duplicating their contracts in the JS API.
Flags precede positional arguments, as in the CLI. Start/serve/stop, session
management, and nested `run` commands are not available through the bridge;
manage the selected session's lifecycle outside the script.

For a popup button observed on the page:

```js
const page = session.page("main");
const { subscription } = await session.command("events", "arm", "--kind", "popup", page.name);
try {
  await page.click("#open-popup");
  const event = await session.command("events", "next", subscription);
  await session.command("popup", "adopt", "--name", "details", event.handle);
  return await session.page("details").title();
} finally {
  await session.command("events", "close", subscription);
}
```

Arm first, trigger once, then consume the event. Do not submit a blocking
`events next` before its triggering command: this runner processes calls
sequentially. Subscription IDs and returned handle IDs are different values.

## Outcomes, timeouts, and ownership

`return` must be JSON-serializable; an omitted/undefined return becomes `null`.
BigInt and cyclic values fail. Successful stdout is the usual
`{"ok":true,"result":...}` envelope. `console` writes to stderr; keep
`process.stdout` reserved for the runner protocol. Imported modules' ordinary
console logs also go to stderr. Avoid logging secrets or form contents.

Driver failures reject with an Error exposing `code`, `mayHaveEffects`,
`operation`, `pageId`, and any partial `result`. A caught failure can be
inspected and reconciled in the script. Uncaught driver errors preserve their
code/partial result; other exceptions use `script_error`. A later workflow
failure conservatively reports earlier possibly mutating operations too.
There are no automatic mutation retries or workflow rollback.

`--timeout` bounds the whole Node execution, startup, and browser calls. The
session's configured SDK command timeout still bounds each browser operation.
Increase both budgets when needed. On cancellation the runner child is reaped
and an active RPC is canceled; already-applied browser actions remain applied.
Finish pages/tasks created solely for the work, or leave them intentionally for
later commands. Running or failing a script does not stop the session.

Scripts can use Node's filesystem/process/network APIs with the user's
permissions. This is a local script runner, not a sandbox for page-supplied
code. Script source is limited to 1 MiB and bridge messages/results to 16 MiB.
The script API is specific to this project; existing Ego Lite scripts are not
automatically compatible.
