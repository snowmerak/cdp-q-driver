# Behavioral bot detection: how automation projects respond

Research date: November 7, 2026. This is a bounded source review for
`cdp-q-driver`, not an exhaustive market survey or a new benchmark. Repository
implementations were inspected at the revisions below. External tools were not
installed or executed. Their advertised detection results remain author claims.
Our earlier measurements are recorded separately in
[automation-signals.md](automation-signals.md).

## Findings

Public projects use several approaches: geometric cursor generators,
recorded-human trajectory replay, correlated keyboard scheduling, and optional
OS input transports. These are separate from browser fingerprint patches.
Adding arbitrary delays does not establish that a session resembles human
input, and passing a fingerprint page does not establish that an interaction
detector accepts the client.

The most consequential finding for our failed diagnostic is that its public
frontend initializes Castle's Browser SDK, obtains a request token, and submits
it with fingerprint/interaction data to the site's backend. The exact backend
mapping to `suspiciousClientSideBehavior` was not available. That flag therefore
does not identify which individual pointer, keyboard, or browser signal failed.

## What the tested page actually exposes

The [interaction page](https://deviceandbrowserinfo.com/are_you_a_bot_interactions)
loads `@castleio/castle-js` through its script bundle and configures it as
`window.cstlxp`. The public
[submission script](https://deviceandbrowserinfo.com/javascripts/bot_results_interactions_visualization.js)
calls `createRequestToken()`, adds the token to the fingerprint payload, and
posts the payload to the backend. It displays the returned verdict and details.
This workflow was confirmed by reading the served HTML and JavaScript; no token
or payload was modified, replayed, or submitted during this review.

Castle's [Browser SDK documentation](https://docs.castle.io/docs/sdk-browser)
explains that these short-lived tokens carry device and behavioral signals for
server-side risk evaluation. Its [bot-detection documentation](https://docs.castle.io/docs/bot-detection)
describes interaction analysis alongside other signals. The diagnostic itself
says it excludes IP reputation and persistent user behavior: the vendor's
general feature set must not be assumed to be enabled in this particular test.

**Known:** the frontend uses Castle and a server verdict. **Unknown:** the
specific model, policy, thresholds, and mapping responsible for our boolean.
The label is not proof that our pointer path alone caused the result.

## Approaches found in public implementations

| Approach | Example and inspected revision | What the implementation does | Evidence limit / cost |
| --- | --- | --- | --- |
| Geometric cursor generation | [Ghost Cursor](https://github.com/Xetera/ghost-cursor/tree/5525a783fcf833a60f1b0746a8f80653ccfee2ba) | Cubic curves, distance/target-size-dependent point generation, landing-point variation, and overshoot/correction options. | Source confirms mechanics, not a guaranteed current Castle verdict. Compatible with a CDP driver in principle. |
| Recorded mouse movement | [Camoufox](https://github.com/daijro/camoufox/tree/8670c2d30f5cb5aeb554ef141c3c1c53cd4cac91) | Selects recorded Cursory trajectories and adapts them to the requested move while preserving timing structure. | Browser integration is substantial; code and data have their own licensing requirements. No local execution here. |
| Correlated typing / movement replay | [human-input](https://github.com/heydryft/human-input/tree/9975cd057329dabfe5dc0f0ca09d6659773ed886) | Uses recorded movement/keystroke data, key-transition classes, paired hold/latency values, and a stable persona. | Its statistical comparisons are project-reported, not an independent live-site benchmark. |
| OS input with a behavior model | [docker-stealthy-auto-browse](https://github.com/psyb0t/docker-stealthy-auto-browse/tree/cabf86556ee5610cb957bb7af05c320b524b4965) | Separates system mouse/keyboard actions from browser automation; adds movement and timing models around system input. | README reports behavioral-test results, but we did not reproduce them. Window focus, desktop access, and platform adapters add operational costs. |

### Cursor generation is more than an eased line

Ghost Cursor's
[movement code](https://github.com/Xetera/ghost-cursor/blob/5525a783fcf833a60f1b0746a8f80653ccfee2ba/src/spoof.ts)
uses target geometry in its path calculation and can choose a point inside an
element rather than always its center. It also contains optional overshoot and
correction behavior. These are concrete differences from our current fixed
12-point, fixed-base-duration eased straight line. Source inspection confirms
these mechanisms; it does not prove that copying them would change our verdict.

Castle's older [mouse-analysis article](https://blog.castle.io/bot-or-not-can-you-spot-the-automated-mouse-movements/)
explicitly discusses detecting sparse movement, simplistic motion, and even
generated curved paths. It is an explanation from 2020, not a specification of
the model running today. The important implication is that a visually smooth
curve is an insufficient test by itself.

### Recorded behavior and correlations

Camoufox's
[cursor implementation](https://github.com/daijro/camoufox/blob/8670c2d30f5cb5aeb554ef141c3c1c53cd4cac91/browser/additions/juggler/input/CursorTrajectory.js)
loads Cursory and generates points/timings from recorded movements. Its current
documentation describes 2,356 recordings. This is a different strategy from
adding independent noise to a mathematical line. The bundled Cursory material
has separate LGPL licensing; adoption requires a proper code/data license
review rather than assuming the whole browser has one uniform license.

The [human-input keyboard implementation](https://github.com/heydryft/human-input/blob/9975cd057329dabfe5dc0f0ca09d6659773ed886/src/keyboard.ts)
selects related hold and latency values from the same recorded keystroke,
conditioned on the transition. Its model allows overlapping key presses and
uses consistent person-specific characteristics. Our current loop always
releases one character before starting the next, so its overlap frequency is
zero. That is a measurable difference; it is not a confirmed explanation for
Castle's decision.

The project's README reports that its parametric and replay models differ under
its own classifier. That result motivates checking joint timing behavior; it
does not establish a commercial-site pass rate.

### OS input is an additional transport, not a human verdict

The reviewed system-input project reports that its modeled OS input passed a
DeviceAndBrowserInfo interaction check while its raw system input did not.
This remains the author's result. Its
[input routing](https://github.com/psyb0t/docker-stealthy-auto-browse/blob/cabf86556ee5610cb957bb7af05c320b524b4965/app/input_actions.py)
does distinguish system movement/click actions from browser-side actions.

Our real-browser tests already observe `isTrusted=true` for native CDP mouse,
keyboard, and input events. Replacing CDP solely to obtain that boolean would
not address a demonstrated gap. An OS backend would instead be a separate
experiment with different focus/coordinate behavior. It would also interact
with the desktop pointer and active window, unlike the current tab-scoped API.

## What detection-side sources support

The research paper
[BeCAPTCHA-Mouse](https://arxiv.org/abs/2005.00890)
studies neuromotor features and synthetic trajectories. Its abstract reports
results on a defined research dataset; it does not supply a current Castle or
Google benchmark. Only the abstract was reviewed for this survey.

The open-source [motion-attestation detector](https://github.com/libcaptcha/motion-attestation)
describes combining movement geometry, click landing/hold behavior, keystroke
timing, scrolling, engagement, and event-order consistency. Its published tests
include multiple generated motion families. This illustrates a multi-channel
detector; it is not evidence that Castle uses its rules.

[Cloudflare's detection documentation](https://developers.cloudflare.com/bots/concepts/bot-detection-engines/)
likewise describes multiple engines and request/session/browser features. A
site's combined decision can therefore differ from a standalone movement
classifier. These sources support separating transport, fingerprint, input,
and session-level observations in our own measurements.

## Assessment for cdp-q-driver

Our existing implementation now has native character/modifier events, bounded
hold/interval jitter, and eased pointer interpolation. The last valid interaction
trial still returned `suspiciousClientSideBehavior=true`, with the separately
displayed webdriver, CDP, mouse-leak, framework-stack, and speed flags false.
This is a small observed result, not a general detection rate.

Remaining differences that can be inspected without maintaining a browser fork:

1. **Measure the baseline first.** Compare a normal manual session and our
   session under the same diagnostic, browser version, viewport, and network.
   A manual false positive would materially change the investigation.
2. **Record delivered events on a local fixture.** Inspect coordinates,
   timestamps, landing offsets, key holds/gaps, overlaps, focus changes, and
   selection/input order. Record timing and geometry rather than typed content.
3. **Treat motion and keyboard models as replaceable experiments.** A
   geometry-aware path or recorded-trace adapter can be assessed independently
   of browser ownership and CDP transport. Each still needs exact-node hit
   checks, frame-coordinate handling, cancellation, and no mutation replay.
4. **Compare distributions and safety, not only one green label.** Repeat
   comparable tests and retain failures. A corpus replay must be evaluated on
   data separate from its source recordings; crossing one hosted test does not
   show generalization.
5. **Keep an OS backend optional if investigated later.** It is a larger
   platform/focus boundary and should not silently replace tab-scoped input.

These are investigation recommendations, not changes implemented by this survey.
The runtime and installed input settings were left unchanged. No conclusion is
made about Ego Lite's behavioral pass rate because it has not been run through
the same controlled comparison.
