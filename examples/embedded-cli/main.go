// Command embedded-cli demonstrates dispatching the complete CLI as `q cdp`.
// Build this example with the desired executable name; no separate driver
// executable is needed, and background serve re-enters this dispatch path.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/snowmerak/cdp-q-driver/cli"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "cdp" {
		_, _ = fmt.Fprintln(os.Stderr, "usage: q cdp [FLAGS] COMMAND ...")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.MainWithOptions(ctx, os.Args[2:], os.Stdin, os.Stdout, os.Stderr, cli.Options{
		CommandPrefix: []string{"cdp"},
	})
	stop()
	os.Exit(code)
}
