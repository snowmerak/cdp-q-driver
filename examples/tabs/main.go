// Command tabs demonstrates a task-owned tab on an existing CDP browser.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	qdriver "github.com/snowmerak/cdp-q-driver"
)

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:9222", "HTTP discovery endpoint or browser WebSocket URL")
	url := flag.String("url", "data:text/html,<title>QDriver demo</title><h1>CDP works</h1>", "URL to open in a task-owned tab")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := run(ctx, *endpoint, *url); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, endpoint, url string) (err error) {
	b, err := qdriver.Attach(ctx, endpoint, qdriver.Options{})
	if err != nil {
		return err
	}
	defer func() {
		_, closeErr := b.Close(ctx)
		err = errors.Join(err, closeErr)
	}()
	task, err := b.NewTask(ctx, "tab example")
	if err != nil {
		return err
	}
	p, err := task.NewPage(ctx)
	if err != nil {
		return err
	}
	if err := p.Goto(ctx, url); err != nil {
		return err
	}
	title, err := p.Title(ctx)
	if err != nil {
		return err
	}
	cleanup, err := task.Finish(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		PageID  string
		Title   string
		Cleanup qdriver.FinishResult
	}{PageID: p.ID(), Title: title, Cleanup: cleanup})
}
