// The events example operates only on task-created fixture pages. Download
// policy affects the default browser context: use a dedicated test browser.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	qdriver "github.com/snowmerak/cdp-q-driver"
)

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:9222", "explicit browser CDP endpoint")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := run(ctx, *endpoint); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, endpoint string) (err error) {
	dir, err := os.MkdirTemp("", "qdriver-events-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file":
			w.Header().Set("Content-Disposition", `attachment; filename="report.txt"`)
			_, _ = fmt.Fprint(w, "report from Chrome")
		case "/popup":
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><title>Owned popup</title><h1>Popup ready</h1>`)
		case "/request":
			_, _ = fmt.Fprint(w, "done")
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = fmt.Fprint(w, `<!doctype html><button id="popup" onclick="window.open('/popup','_blank')">Open</button><button id="prompt" onclick="window.answer=prompt('Your name?')">Prompt</button><input hidden type="file" id="upload"><a id="download" href="/file">Download</a><button id="request" onclick="fetch('/request')">Request</button>`)
		}
	}))
	defer server.Close()
	b, err := qdriver.Attach(ctx, endpoint, qdriver.Options{})
	if err != nil {
		return err
	}
	defer func() { _, closeErr := b.Close(ctx); err = errors.Join(err, closeErr) }()
	task, err := b.NewTask(ctx, "events and files example")
	if err != nil {
		return err
	}
	p, err := task.NewPage(ctx)
	if err != nil {
		return err
	}
	if err := p.Goto(ctx, server.URL); err != nil {
		return err
	}

	// Arm before the trigger. Expected popups become task-owned; observing
	// an arbitrary popup with Subscribe alone does not claim it.
	popups, err := p.ExpectPopup(ctx)
	if err != nil {
		return err
	}
	defer popups.Close()
	if _, err := p.Click(ctx, qdriver.CSS("#popup")); err != nil {
		return err
	}
	opened, err := popups.Next(ctx)
	if err != nil {
		return err
	}
	popup, err := opened.Popup.Page(ctx)
	if err != nil {
		return err
	}
	if err := popup.WaitForSelector(ctx, qdriver.CSS("h1")); err != nil {
		return err
	}
	popupTitle, err := popup.Title(ctx)
	if err != nil {
		return err
	}

	dialogs, err := p.ExpectDialog(ctx)
	if err != nil {
		return err
	}
	defer dialogs.Close()
	if _, err := p.Click(ctx, qdriver.CSS("#prompt")); err != nil && !errors.Is(err, qdriver.ErrDialogOpen) {
		return err
	}
	modal, err := dialogs.Next(ctx)
	if err != nil {
		return err
	}
	if err := modal.Dialog.Accept(ctx, "Ada"); err != nil {
		return err
	}
	if err := p.WaitForFunction(ctx, `window.answer==='Ada'`); err != nil {
		return err
	}

	upload := filepath.Join(dir, "upload.txt")
	if err := os.WriteFile(upload, []byte("upload from Go"), 0600); err != nil {
		return err
	}
	if _, err := p.SetInputFiles(ctx, qdriver.CSS("#upload"), upload); err != nil {
		return err
	}
	if err := p.WaitForFunction(ctx, `document.querySelector('#upload').files[0].name==='upload.txt'`); err != nil {
		return err
	}

	// This is an explicit default-context policy change, reset to default by
	// Close. GUID filenames are copied to an explicit, nonexisting destination.
	if err := b.EnableDownloads(ctx, filepath.Join(dir, "downloads")); err != nil {
		return err
	}
	downloads, err := p.ExpectDownload(ctx)
	if err != nil {
		return err
	}
	defer downloads.Close()
	if _, err := p.Click(ctx, qdriver.CSS("#download")); err != nil {
		return err
	}
	transfer, err := downloads.Next(ctx)
	if err != nil {
		return err
	}
	saved := filepath.Join(dir, "saved.txt")
	if err := transfer.Download.SaveAs(ctx, saved); err != nil {
		return err
	}
	contents, err := os.ReadFile(saved)
	if err != nil {
		return err
	}

	idle, err := p.ExpectNetworkIdle(ctx, 100*time.Millisecond)
	if err != nil {
		return err
	}
	if _, err := p.Click(ctx, qdriver.CSS("#request")); err != nil {
		return err
	}
	if err := idle.Wait(ctx); err != nil {
		return err
	}
	cleanup, err := task.Finish(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		PopupTitle, Prompt, Upload, Download string
		Cleanup                              qdriver.FinishResult
	}{popupTitle, "Ada", "upload.txt", string(contents), cleanup})
}
