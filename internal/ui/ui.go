// Package ui holds hand-rolled ANSI styling helpers.
package ui

import (
	"fmt"
	"os"
	"strings"
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	blue    = "\x1b[34m"
	magenta = "\x1b[35m"
	cyan    = "\x1b[36m"
)

// Palette applies colours when enabled and is a no-op otherwise.
type Palette struct {
	Enabled bool
}

// Auto enables colours when stdout is a terminal and NO_COLOR is unset.
func Auto() Palette {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return Palette{}
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return Palette{}
	}
	return Palette{Enabled: fi.Mode()&os.ModeCharDevice != 0}
}

func (p Palette) wrap(code, s string) string {
	if !p.Enabled {
		return s
	}
	return code + s + reset
}

func (p Palette) Bold(s string) string    { return p.wrap(bold, s) }
func (p Palette) Dim(s string) string     { return p.wrap(dim, s) }
func (p Palette) Red(s string) string     { return p.wrap(red, s) }
func (p Palette) Green(s string) string   { return p.wrap(green, s) }
func (p Palette) Yellow(s string) string  { return p.wrap(yellow, s) }
func (p Palette) Blue(s string) string    { return p.wrap(blue, s) }
func (p Palette) Magenta(s string) string { return p.wrap(magenta, s) }
func (p Palette) Cyan(s string) string    { return p.wrap(cyan, s) }

// HPBar renders e.g. "[██████····] 12/20", green/yellow/red by ratio.
func (p Palette) HPBar(cur, maxHP, width int) string {
	if maxHP <= 0 {
		maxHP = 1
	}
	cur = min(max(cur, 0), maxHP)
	filled := cur * width / maxHP
	if cur > 0 && filled == 0 {
		filled = 1
	}
	ratio := float64(cur) / float64(maxHP)
	color := p.Green
	switch {
	case ratio <= 0.25:
		color = p.Red
	case ratio <= 0.5:
		color = p.Yellow
	}
	bar := color(strings.Repeat("█", filled)) + p.Dim(strings.Repeat("·", width-filled))
	return fmt.Sprintf("[%s] %s", bar, color(fmt.Sprintf("%d/%d", cur, maxHP)))
}

// Danger renders a danger level as skulls, coloured by severity.
func (p Palette) Danger(level int) string {
	if level <= 0 {
		return p.Green("safe-ish")
	}
	s := strings.Repeat("☠", level)
	switch {
	case level >= 4:
		return p.Magenta(s)
	case level == 3:
		return p.Red(s)
	case level == 2:
		return p.Yellow(s)
	default:
		return p.Green(s)
	}
}
