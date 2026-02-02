//go:build unix || darwin

package main

import (
	"os"
	"strings"

	"github.com/julez-dev/chatuino/save"
	"golang.org/x/sys/unix"
)

func hasImageSupport(mode save.GraphicsMode) bool {
	_, isKitty := os.LookupEnv("KITTY_WINDOW_ID") // always defined by kitty
	term := os.Getenv("TERM")

	switch mode {
	case save.GraphicsModeSixel:
		// Sixel is supported by various terminals
		// Common sixel-capable terminals include: mlterm, xterm (with sixel), mintty, WezTerm, foot
		return hasSixelSupport()
	case save.GraphicsModeKitty:
		fallthrough
	default:
		return isKitty || term == "xterm-ghostty"
	}
}

func hasSixelSupport() bool {
	// Check common environment indicators for sixel support
	term := os.Getenv("TERM")
	termProgram := os.Getenv("TERM_PROGRAM")

	// Known sixel-supporting terminals
	sixelTerms := []string{
		"mlterm",
		"xterm-256color", // xterm with sixel support
		"mintty",
		"foot",
	}

	for _, t := range sixelTerms {
		if strings.Contains(term, t) {
			return true
		}
	}

	// Check for WezTerm
	if termProgram == "WezTerm" {
		return true
	}

	// Check for foot terminal
	if strings.HasPrefix(term, "foot") {
		return true
	}

	// When user explicitly sets sixel mode in settings, we trust they know their terminal supports it.
	// This allows advanced users to use sixel on terminals we don't explicitly detect.
	// If sixel doesn't work, the user can switch back to text mode or kitty mode.
	return true
}

func getTermCellWidthHeight() (float32, float32, error) {
	f, err := os.OpenFile("/dev/tty", unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NDELAY|unix.O_RDWR, 0666)
	if err != nil {
		return 0, 0, err
	}

	sz, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)

	if err != nil {
		return 0, 0, err
	}

	cellWidth := float32(sz.Xpixel) / float32(sz.Col)
	cellHeight := float32(sz.Ypixel) / float32(sz.Row)

	return cellWidth, cellHeight, nil
}
