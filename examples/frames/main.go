// Command frames demonstrates iframe refs and open shadow-root input.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
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
	defer func() { _, closeErr := b.Close(ctx); err = errors.Join(err, closeErr) }()
	task, err := b.NewTask(ctx, "frames example")
	if err != nil {
		return err
	}
	p, err := task.NewPage(ctx)
	if err != nil {
		return err
	}
	child := `<!doctype html><label for="name">Frame name</label><input id="name"><button onclick="parent.postMessage(document.getElementById('name').value,'*')">Frame save</button>`
	source := `<!doctype html><title>QDriver frames</title><output id="result"></output><iframe name="details" srcdoc="` + html.EscapeString(child) + `"></iframe><div id="host"></div><script>addEventListener('message',e=>document.getElementById('result').textContent=e.data);const root=document.getElementById('host').attachShadow({mode:'open'});root.innerHTML='<button>Shadow ready</button>';root.querySelector('button').onclick=()=>document.body.dataset.shadow='ready'</script>`
	if err := p.Goto(ctx, "data:text/html;charset=utf-8,"+url.PathEscape(source)); err != nil {
		return err
	}
	snapshot, err := p.Snapshot(ctx, qdriver.SnapshotOptions{Scope: qdriver.FullPage})
	if err != nil {
		return err
	}
	var name, save qdriver.Ref
	for _, node := range snapshot.Nodes {
		if node.Role == "textbox" && node.Name == "Frame name" {
			name = node.Ref
		}
		if node.Role == "button" && node.Name == "Frame save" {
			save = node.Ref
		}
	}
	if name.String() == "" || save.String() == "" {
		return errors.New("iframe controls missing from snapshot")
	}
	if _, err := p.Fill(ctx, qdriver.ByRef(name), "Go frames work"); err != nil {
		return err
	}
	if _, err := p.Click(ctx, qdriver.ByRef(save)); err != nil {
		return err
	}
	if _, err := p.Click(ctx, qdriver.Role("button", "Shadow ready")); err != nil {
		return err
	}
	if err := p.WaitForFunction(ctx, `document.getElementById('result').textContent==='Go frames work'&&document.body.dataset.shadow==='ready'`); err != nil {
		return err
	}
	frames, err := p.Frames(ctx)
	if err != nil {
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
		Frames  []qdriver.FrameInfo
		Result  json.RawMessage
		Cleanup qdriver.FinishResult
	}{Frames: frames, Result: value, Cleanup: cleanup})
}
