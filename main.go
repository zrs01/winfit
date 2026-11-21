// Package main is the entry point of the application.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/urfave/cli/v3"
)

// version is overridden at build time via
// -ldflags "-X main.version=<value>" (see the Makefile). The "dev" default
// ensures the --version flag always exists even on a plain `go build`.
var version = "dev"

func main() {
	setupLogging()

	cmd := &cli.Command{
		Name:    "winfit",
		Usage:   "Position and resize application windows on Windows 10/11",
		Version: version,
		Flags:   moveFlags(),
		Action:  runMove,
		Commands: []*cli.Command{
			{
				Name:   "get",
				Usage:  "Print a target window's position and size (same target flags as a move)",
				Flags:  getFlags(),
				Action: runGet,
			},
			{
				Name:   "list",
				Usage:  "Print candidate windows (HWND PID APP CLASS TITLE) in z-order",
				Flags:  listFlags(),
				Action: runList,
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		if ec, ok := errors.AsType[cli.ExitCoder](err); ok {
			os.Exit(ec.ExitCode())
		}
		slog.Error(err.Error())
		// Debug level shows the wrapped cause chain (the replacement for a
		// stack dump: stdlib errors carry the chain, not the frames).
		for cause := errors.Unwrap(err); cause != nil; cause = errors.Unwrap(cause) {
			slog.Debug(fmt.Sprintf("caused by: %v", cause))
		}
		os.Exit(1)
	}
}
