package main

import (
	"fmt"

	"github.com/zrs01/winfit/internal/place"

	"github.com/urfave/cli/v3"
)

// moveFlags are the root command's position/size and selector flags.
func moveFlags() []cli.Flag {
	pos := func(name, alias, usage string) cli.Flag {
		return &cli.StringFlag{Name: name, Aliases: []string{alias}, Usage: usage}
	}
	flags := []cli.Flag{
		pos("top", "t", "position from top edge: px or % of work-area height"),
		pos("bottom", "b", "position from bottom edge: px or %"),
		pos("left", "l", "position from left edge: px or % of work-area width"),
		pos("right", "r", "position from right edge: px or %"),
		&cli.StringFlag{Name: "width", Aliases: []string{"w"}, Usage: "window width: px or % of work area"},
		&cli.StringFlag{Name: "height", Aliases: []string{"H"}, Usage: "window height: px or % (capital -H; -h is help)"},
		&cli.BoolFlag{Name: "keep-aspect", Aliases: []string{"k"}, Usage: "preserve current aspect ratio on resize"},
	}
	flags = append(flags, selectorFlags()...)
	return append(flags, &cli.StringFlag{Name: "monitor", Usage: "reference monitor: 1-based index or display name"})
}

// selectorFlags are the target-selection flags, shared by every subcommand
// that resolves a window (root move, get).
func selectorFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "hwnd", Usage: "target this exact window handle (hex, from 'winfit list')"},
		&cli.StringFlag{Name: "title", Usage: "target window whose title contains this"},
		&cli.StringFlag{Name: "app", Usage: "target window whose executable name contains this"},
		&cli.BoolFlag{Name: "topmost", Usage: "among several matches, pick the frontmost"},
	}
}

// getFlags are `winfit get`'s flags: the selectors plus machine-readable
// output. No geometry flags — get never moves anything.
func getFlags() []cli.Flag {
	return append(selectorFlags(),
		&cli.BoolFlag{Name: "json", Usage: "emit one JSON object on stdout"},
	)
}

// specFromFlags parses the position/size flags into a place.Spec.
func specFromFlags(c *cli.Command) (place.Spec, error) {
	var s place.Spec
	pairs := []struct {
		flag string
		size bool
		dst  **place.Value
	}{
		{"top", false, &s.Top},
		{"bottom", false, &s.Bottom},
		{"left", false, &s.Left},
		{"right", false, &s.Right},
		{"width", true, &s.Width},
		{"height", true, &s.Height},
	}
	for _, p := range pairs {
		if raw := c.String(p.flag); raw != "" {
			v, err := place.ParseValue(raw, p.size)
			if err != nil {
				return s, fmt.Errorf("--%s: %w", p.flag, err)
			}
			*p.dst = &v
		}
	}
	s.KeepAspect = c.Bool("keep-aspect")
	return s, nil
}

// listFlags narrow `winfit list` output; same semantics as the root
// selectors.
func listFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{Name: "title", Usage: "only windows whose title contains this"},
		&cli.StringFlag{Name: "app", Usage: "only windows whose executable name contains this"},
	}
}
