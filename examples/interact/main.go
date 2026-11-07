// Command interact demonstrates snapshot -> ref -> input -> verification.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	qdriver "github.com/snowmerak/cdp-q-driver"
)

func main() {
	endpoint := flag.String("endpoint", "http://127.0.0.1:9222", "HTTP discovery endpoint or browser WebSocket URL")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := run(ctx, *endpoint); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, endpoint string) (err error) {
	b, err := qdriver.Attach(ctx, endpoint, qdriver.Options{})
	if err != nil {
		return err
	}
	defer func() {
		_, closeErr := b.Close(ctx)
		err = errors.Join(err, closeErr)
	}()
	task, err := b.NewTask(ctx, "snapshot input example")
	if err != nil {
		return err
	}
	p, err := task.NewPage(ctx)
	if err != nil {
		return err
	}
	html := `<!doctype html><title>QDriver interaction</title><label for="name">Name</label><input id="name"><button onclick="document.getElementById('result').textContent=document.getElementById('name').value">Save</button><output id="result"></output>`
	if err := p.Goto(ctx, "data:text/html;charset=utf-8,"+url.PathEscape(html)); err != nil {
		return err
	}
	snapshot, err := p.Snapshot(ctx, qdriver.SnapshotOptions{})
	if err != nil {
		return err
	}
	var name, save qdriver.Ref
	for _, node := range snapshot.Nodes {
		if node.Role == "textbox" && node.Name == "Name" {
			name = node.Ref
		}
		if node.Role == "button" && node.Name == "Save" {
			save = node.Ref
		}
	}
	if name.String() == "" || save.String() == "" {
		return errors.New("fixture controls were not present in the snapshot")
	}
	if _, err := p.Fill(ctx, qdriver.ByRef(name), "Go CDP works"); err != nil {
		return err
	}
	if _, err := p.Click(ctx, qdriver.ByRef(save)); err != nil {
		return err
	}
	if err := p.WaitForFunction(ctx, `document.getElementById('result').textContent==='Go CDP works'`); err != nil {
		return err
	}
	value, err := p.Evaluate(ctx, `document.getElementById('result').textContent`)
	if err != nil {
		return err
	}
	cleanup, err := task.Finish(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Snapshot string
		Result   json.RawMessage
		Cleanup  qdriver.FinishResult
	}{Snapshot: snapshot.Content, Result: value, Cleanup: cleanup})
}
