package qdriver

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

//go:embed scripts/element.js
var elementFunction string

// ActionReceipt reports input dispatch, not application success. InputStarted
// means a CDP input command may have been sent, including a pointer move.
// InputCompleted means the entire input sequence was acknowledged.
type ActionReceipt struct {
	InputStarted   bool
	InputCompleted bool
}

type elementState struct {
	Connected, Supported, Visible, Enabled, Editable, Focused, Hit bool
	X, Y                                                           float64
	Value                                                          string
}

func (p *Page) inspectElement(ctx context.Context, _ *chromedp.Target, node resolvedElement, action string) (elementState, error) {
	state, err := nodeCall[elementState](ctx, node.session, node.object, elementFunction, action)
	if err != nil {
		return state, err
	}
	if !state.Connected {
		return state, ErrStaleRef
	}
	if !state.Supported {
		return state, ErrUnsupported
	}
	if state.Visible {
		x, y, visible, hit, err := p.inputPoint(ctx, node.frame, state.X, state.Y)
		if err != nil {
			return state, err
		}
		state.X, state.Y, state.Visible, state.Hit = x, y, state.Visible && visible, state.Hit && hit
	}
	return state, nil
}

func actionable(state elementState, editing, hit bool) error {
	if !state.Visible || !state.Enabled || (hit && !state.Hit) {
		return fmt.Errorf("%w (visible=%t enabled=%t hit=%t)", ErrNotActionable, state.Visible, state.Enabled, state.Hit)
	}
	if editing && !state.Editable {
		return ErrNotEditable
	}
	return nil
}

func (p *Page) sameDocument(ctx context.Context, t *chromedp.Target, node resolvedElement) error {
	_, err := p.refreshFrames(ctx, t)
	if err != nil {
		return err
	}
	frame := p.frames[node.frame]
	if frame == nil || frame.doc.key != node.document {
		return ErrDocumentChanged
	}
	return nil
}

// Click sends one left-button click to the exact selected node, with visibility,
// enabled, hit-target, and post-hover checks. It never retries dispatched input.
func (p *Page) Click(ctx context.Context, locator Locator) (ActionReceipt, error) {
	return p.inputAction(ctx, "click", locator, "", keyStroke{})
}

// Fill replaces a text input, textarea, or plain contenteditable host's contents
// using native text input. It requires the requested text to remain afterwards;
// application formatting can produce ErrFillRejected after an actual edit.
func (p *Page) Fill(ctx context.Context, locator Locator, text string) (ActionReceipt, error) {
	return p.inputAction(ctx, "fill", locator, text, keyStroke{})
}

// Press focuses an element and sends a key or modifier chord, for example Enter,
// Tab, ArrowLeft, or Control+A. Rich-editor-specific keyboard behavior is outside
// this milestone. Unsupported keys are rejected before changing the page.
func (p *Page) Press(ctx context.Context, locator Locator, key string) (ActionReceipt, error) {
	stroke, err := parseKey(key)
	if err != nil {
		return ActionReceipt{}, err
	}
	return p.inputAction(ctx, "press", locator, "", stroke)
}

func (p *Page) inputAction(caller context.Context, kind string, locator Locator, text string, key keyStroke) (ActionReceipt, error) {
	return runPageOperation(caller, p, kind, func(ctx context.Context, t *chromedp.Target, mark func()) (ActionReceipt, error) {
		var receipt ActionReceipt
		node, err := p.resolve(ctx, t, locator)
		if err != nil {
			return receipt, err
		}
		defer p.releaseObject(ctx, node.session, node.object)
		// Chromium's native input must target the active tab; background tabs
		// can otherwise stop acknowledging pointer actions or animation work.
		mark()
		if _, err := cdp.Call(ctx, p.task.browser.session(), target.ActivateTarget, target.ActivateTargetParams{TargetID: target.ID(p.id)}); err != nil {
			return receipt, err
		}
		state, err := p.inspectElement(ctx, t, node, "inspect")
		if err != nil {
			return receipt, err
		}
		if err := actionable(state, kind == "fill", false); err != nil {
			return receipt, err
		}
		if err := p.sameDocument(ctx, t, node); err != nil {
			return receipt, err
		}
		mark() // Scrolling/focusing can itself change the page, before CDP input.
		if err := p.scrollFrameOwners(ctx, node.frame); err != nil {
			return receipt, err
		}
		state, err = p.inspectElement(ctx, t, node, "scroll")
		if err != nil {
			return receipt, err
		}
		if err := actionable(state, kind == "fill", true); err != nil {
			return receipt, err
		}
		if kind == "click" {
			return p.clickElement(ctx, t, node, state, receipt)
		}
		action := "focus"
		if kind == "fill" {
			action = "fill"
		}
		state, err = p.inspectElement(ctx, t, node, action)
		if err != nil {
			return receipt, err
		}
		if err := actionable(state, kind == "fill", true); err != nil {
			return receipt, err
		}
		if !state.Focused {
			return receipt, ErrNotActionable
		}
		if err := p.sameDocument(ctx, t, node); err != nil {
			return receipt, err
		}
		// Focus/selection handlers may move focus after the preparation call.
		// Recheck the exact control immediately before native keyboard input.
		state, err = p.inspectElement(ctx, t, node, "inspect")
		if err != nil {
			return receipt, err
		}
		if err := actionable(state, kind == "fill", true); err != nil {
			return receipt, err
		}
		if !state.Focused {
			return receipt, ErrNotActionable
		}
		receipt.InputStarted = true
		if kind == "fill" {
			_, err = cdp.Call(ctx, node.session, input.InsertText, input.InsertTextParams{Text: text})
		} else {
			err = p.dispatchKey(ctx, node.session, key)
		}
		if err != nil {
			return receipt, err
		}
		receipt.InputCompleted = true
		if kind == "fill" {
			if err := p.sameDocument(ctx, t, node); err != nil {
				return receipt, err
			}
			state, err = p.inspectElement(ctx, t, node, "inspect")
			if err == nil && state.Value != text {
				err = ErrFillRejected
			}
		}
		return receipt, err
	})
}

func (p *Page) clickElement(ctx context.Context, t *chromedp.Target, node resolvedElement, state elementState, receipt ActionReceipt) (ActionReceipt, error) {
	receipt.InputStarted = true
	_, err := cdp.Call(ctx, node.session, input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseMoved, X: state.X, Y: state.Y})
	if err != nil {
		return receipt, err
	}
	// Hover handlers can insert an overlay or move the target. Fail rather than
	// clicking its old location or retrying a partially dispatched gesture.
	after, err := p.inspectElement(ctx, t, node, "inspect")
	if err != nil {
		return receipt, err
	}
	if err := actionable(after, false, true); err != nil {
		return receipt, err
	}
	if math.Abs(state.X-after.X) > 1 || math.Abs(state.Y-after.Y) > 1 {
		if p.frames[node.frame].info.ParentID == "" {
			return receipt, ErrNotActionable
		}
		// Entering a same-process iframe can make Chromium adjust an outer
		// scroll position. Reposition before the first press, without replaying
		// a click or accepting a different node.
		if _, err := cdp.Call(ctx, node.session, input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseMoved, X: after.X, Y: after.Y}); err != nil {
			return receipt, err
		}
	}
	if err := p.sameDocument(ctx, t, node); err != nil {
		return receipt, err
	}
	final, err := p.inspectElement(ctx, t, node, "inspect")
	if err != nil {
		return receipt, err
	}
	if err := actionable(final, false, true); err != nil {
		return receipt, err
	}
	if math.Abs(after.X-final.X) > 1 || math.Abs(after.Y-final.Y) > 1 {
		return receipt, ErrNotActionable
	}
	press := input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMousePressed, X: final.X, Y: final.Y, Button: input.MouseButtonLeft, Buttons: 1, ClickCount: 1}
	_, err = cdp.Call(ctx, node.session, input.DispatchMouseEvent, press)
	if err != nil {
		p.releaseMouse(ctx, node.session, press)
		return receipt, err
	}
	press.Type, press.Buttons = input.DispatchMouseEventTypeMouseReleased, 0
	if err := ctx.Err(); err != nil {
		p.releaseMouse(ctx, node.session, press)
		return receipt, err
	}
	if _, err := cdp.Call(ctx, node.session, input.DispatchMouseEvent, press); err != nil {
		return receipt, err
	}
	receipt.InputCompleted = true
	return receipt, nil
}

func (p *Page) releaseMouse(ctx context.Context, t cdp.Session, event input.DispatchMouseEventParams) {
	// Complete a possibly pressed gesture once, without replaying the press.
	// A release may still cause the intended click after the caller's timeout.
	event.Type, event.Buttons = input.DispatchMouseEventTypeMouseReleased, 0
	if p.deferForDialog(func(cleanup context.Context) { _, _ = cdp.Call(cleanup, t, input.DispatchMouseEvent, event) }) {
		return
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.task.browser.options.CleanupTimeout)
	defer cancel()
	event.Type, event.Buttons = input.DispatchMouseEventTypeMouseReleased, 0
	_, _ = cdp.Call(cleanup, t, input.DispatchMouseEvent, event)
}

type keyStroke struct {
	key, code, text string
	virtual         int64
	modifiers       int64
}

func parseKey(chord string) (keyStroke, error) {
	parts := strings.Split(chord, "+")
	var key keyStroke
	for _, part := range parts[:len(parts)-1] {
		switch part {
		case "Alt":
			key.modifiers |= 1
		case "Control":
			key.modifiers |= 2
		case "Meta":
			key.modifiers |= 4
		case "Shift":
			key.modifiers |= 8
		default:
			return keyStroke{}, ErrUnsupported
		}
	}
	name := parts[len(parts)-1]
	keys := map[string]int64{"Enter": 13, "Tab": 9, "Escape": 27, "Backspace": 8, "Delete": 46, "ArrowLeft": 37, "ArrowUp": 38, "ArrowRight": 39, "ArrowDown": 40, "Home": 36, "End": 35, "PageUp": 33, "PageDown": 34}
	if virtual, ok := keys[name]; ok {
		key.key, key.code, key.virtual = name, name, virtual
		if name == "Enter" && key.modifiers&7 == 0 {
			key.text = "\r"
		}
	} else if utf8.RuneCountInString(name) == 1 && unicode.IsPrint([]rune(name)[0]) {
		key.key = name
		r := []rune(name)[0]
		if key.modifiers&8 != 0 && unicode.IsLetter(r) {
			key.key = strings.ToUpper(name)
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			key.code, key.virtual = "Key"+strings.ToUpper(name), int64(unicode.ToUpper(r))
		} else if r >= '0' && r <= '9' {
			key.code, key.virtual = "Digit"+name, int64(r)
		}
		if key.modifiers&7 == 0 {
			key.text = key.key
		}
	} else {
		return keyStroke{}, ErrUnsupported
	}
	return key, nil
}

func (p *Page) dispatchKey(ctx context.Context, t cdp.Session, key keyStroke) error {
	params := input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyDown, Key: key.key, Code: key.code, Text: key.text, Modifiers: key.modifiers, WindowsVirtualKeyCode: key.virtual}
	_, err := cdp.Call(ctx, t, input.DispatchKeyEvent, params)
	params.Type, params.Text = input.DispatchKeyEventTypeKeyUp, ""
	if err != nil || ctx.Err() != nil {
		if p.deferForDialog(func(cleanup context.Context) { _, _ = cdp.Call(cleanup, t, input.DispatchKeyEvent, params) }) {
			return errors.Join(err, ctx.Err())
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.task.browser.options.CleanupTimeout)
		defer cancel()
		_, _ = cdp.Call(cleanup, t, input.DispatchKeyEvent, params)
		if err != nil {
			return err
		}
		return ctx.Err()
	}
	_, err = cdp.Call(ctx, t, input.DispatchKeyEvent, params)
	return err
}
