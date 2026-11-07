# Automation signals: measurements and limits

Measured on November 7, 2026, using Windows/ARM64, Go 1.27.3, Chrome
156.0.8078.80, and CDP 1.3. These are small diagnostic samples, not a general
bot-detection rate or an Ego Lite comparison.

## Changes

The baseline was commit `2fc47e7`, with managed launch using
`--remote-debugging-port=0`. The updated implementation selects a nonzero
loopback port and waits for the browser WebSocket URL in its own child's fresh
log. This avoids using an unrelated service that might win the brief port-bind
race. The default remains a visible browser with a dedicated persistent profile.

Chrome's port-zero switch is a documented cause of `navigator.webdriver=true`;
see [MDN's browser API documentation](https://developer.mozilla.org/en-US/docs/Web/API/Navigator/webdriver).
Browser versions differ: the tested Chrome also returned false in headless mode
with a nonzero port, so the driver does not rely on that property to determine
whether headless is detectable.

The SDK now sends `Runtime.disable` after initializing each top-level or OOPIF
session. Explicit evaluation and isolated-world creation do not need continued
Runtime event reporting. This reduces console-related CDP observation while
preserving snapshots, refs, native input, navigation, dialogs, network events,
and file workflows in the integration suite. Runtime is still enabled briefly
during chromedp initialization. Adopting an already active document or attaching
to a running OOPIF can therefore expose that initialization; this change does
not make the connection invisible. See the [Runtime protocol](https://chromedevtools.github.io/devtools-protocol/tot/Runtime/).

No browser-property getters, User-Agent, WebGL, client hints, device values, or
detector results were overridden. Both changes were tested together; these
measurements do not isolate their individual contributions.

## Public diagnostics

The same dedicated profile was reused between the headed baseline and updated
fingerprint checks. Each browser was stopped after its trial/group. Pages were
read through the actual CLI; values below are live results, not the site's
static HTML template.

| Test | Samples | `webdriver` | CDP flag | Overall verdict |
| --- | ---: | --- | --- | --- |
| Baseline, visible, fingerprint page | 1 | true | true | Bot |
| Updated, visible, fingerprint page | 3 | false | false | Human in all 3 |
| Updated, headless, fingerprint page | 1 | false | false | Bot: `hasBotUserAgent` |
| Updated, visible, interaction page, Fill + Click | 1 | false | false | Bot: `suspiciousClientSideBehavior` |
| Updated, visible, interaction page, Click + per-character Press | 1 valid submission | false | false | Bot: `suspiciousClientSideBehavior` |
| Updated, visible, Google search | 1 | false | Not measured | Search results loaded; no challenge observed |
| Paced Fill + interpolated field/submit clicks, visible | 1 valid submission | false | false | Bot: `suspiciousClientSideBehavior` |
| Bezier pointer + QWERTY transition/rollover Fill, visible | 1 valid submission | false | false | Bot: `suspiciousClientSideBehavior` |

The [fingerprint diagnostic](https://deviceandbrowserinfo.com/are_you_a_bot)
states that it tests browser fingerprinting and HTTP headers, without IP
reputation or input behavior. All reported detail flags were false in the three
updated visible samples. The single updated headless sample was flagged only
for its User-Agent. Its native `webdriver` getter was retained.

The [interaction diagnostic](https://deviceandbrowserinfo.com/are_you_a_bot_interactions)
explicitly permits random email/password submissions. Only synthetic test data
was entered. For both valid updated interaction samples,
`suspiciousClientSideBehavior=true`, while `superHumanSpeed`, `hasCDPMouseLeak`,
`hasAutomationFrameworkStackTrace`, webdriver, and CDP flags were false.
Per-character key events alone did not remove the behavioral verdict.

A later implementation added eased pointer interpolation and made Fill emit
physical US-layout key/modifier down/up events with bounded timing jitter.
Defaults: 12 mouse points over 180 ms, 40 ms button hold, 35 ms key hold, 65 ms
between characters, and ±15 ms duration jitter. One valid test used this paced
Fill and interpolated clicks on both fields and the submit button. Its overall
verdict remained bot with only `suspiciousClientSideBehavior=true`;
`superHumanSpeed`, mouse-leak, framework-stack, webdriver, and CDP flags stayed
false. This is evidence of a remaining behavioral limit, not a passed test.

The next implementation replaced the default line with an eased cubic Bezier
curve, adjusted movement duration by distance and visible target size, and
added QWERTY hand/finger/row/distance/Shift transition costs. Eligible
opposite-hand transitions have a 15% chance of planned overlap, capped at two
held character keys. These are original heuristics, not a learned or recorded
human model. Endpoints remain exact; overshoot, random landing, and real IME
composition are not implemented. One valid visible-browser submission used
the same dedicated profile and synthetic credentials. Form validity was
checked before submission. It still returned only
`suspiciousClientSideBehavior=true`; all other reported flags were false.
This single observation demonstrates no improvement in the overall verdict.
It does not isolate cursor and keyboard contributions or reveal the server's
decision rule. An earlier concurrent attempt had a canceled click and was
excluded. Raw output is in the ignored local diagnostic artifact
`bin/automation-interactions/modeled-fill.json`.

Stopping this trial exceeded the owned-browser shutdown budget and retained
`browser.lock`. Both recorded Chrome/control PIDs were subsequently confirmed
absent before that exact lock was removed. No trial process was left running;
this cleanup warning is separate from the detector verdict.

Pure logic tests and local Chrome fixtures separately verify exact text, curved path geometry,
distance/target-size timing, physical-key layout coverage, overlapping native
key events, flushing before Unicode fallback, and cancellation releasing both
held character keys and Shift without replay. The full browser regression
suite passed after this extension (95.497 seconds on Windows/ARM64).

Two earlier keyboard attempts did not submit because the password failed the
form's native pattern/length validation. They produced no detector result and
are excluded from the table. The valid keyboard attempt checked form validity
before submitting. One earlier fingerprint observation used a wait predicate
that matched only the bot wording; it timed out without capturing a verdict and
is also excluded.

One Google search for `golang context documentation` loaded the normal search
URL and title, with ten result-page `h3` headings. At observation time it was
not on `/sorry/` and no reCAPTCHA iframe was present. This was a single
exploratory request, not a controlled Google before/after comparison. It does
not establish that all Google challenges are absent or that the driver's
changes alone caused the outcome.

## Practical implications

- Prefer the visible default and reuse the dedicated profile when appropriate.
- A passed fingerprint test does not mean the site's input model accepts the
  driver. Fill, Press, and CDP mouse input remain automated actions.
- Headless remains distinguishable even when `webdriver` is false.
- The single Google result does not establish a CAPTCHA pass rate. IP
  reputation, account-history checks, and other services' risk models were not
  measured. A service challenge remains an observed outcome to report.

The real-browser regression suite passed with visible and headless CLI coverage,
including iframe/OOPIF navigation and native input. Build, vet, lint, and
govulncheck also passed. The headed CLI test checks that `webdriver` is false;
the headless test checks that the native getter remains unmodified. Startup-log
parser tests cover mismatched ports, non-browser targets, malformed addresses,
and bounded reads of large logs.
