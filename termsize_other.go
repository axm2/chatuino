//go:build !unix && !darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/julez-dev/chatuino/save"
)

var errUnsupported = errors.New("image support not available for this platform")

func hasImageSupport(mode save.GraphicsMode) bool {
	// On Windows, only sixel mode is supported (e.g., Windows Terminal with sixel support)
	if mode == save.GraphicsModeSixel {
		return hasSixelSupport()
	}
	// Kitty graphics protocol is not supported on Windows
	return false
}

func hasSixelSupport() bool {
	// Windows Terminal and some other Windows terminals support sixel
	// Check for Windows Terminal via WT_SESSION environment variable
	_, isWindowsTerminal := os.LookupEnv("WT_SESSION")
	if isWindowsTerminal {
		return true
	}

	// Check for mintty (Git Bash, Cygwin, MSYS2)
	term := os.Getenv("TERM")
	if strings.Contains(term, "mintty") || strings.Contains(term, "xterm") {
		return true
	}

	// When user explicitly sets sixel mode in settings, we trust they know their terminal supports it.
	// This allows advanced users to use sixel on terminals we don't explicitly detect.
	// If sixel doesn't work, the user can switch back to text mode.
	return true
}

func getTermCellWidthHeight() (float32, float32, error) {
	// Try to get terminal size on Windows using PowerShell or mode command
	// This is a best-effort approach

	// First try using mode command (available in cmd.exe)
	cmd := exec.Command("cmd", "/c", "mode", "con")
	output, err := cmd.Output()
	if err == nil {
		// Parse mode output to get columns and lines
		lines := strings.Split(string(output), "\n")
		var cols, rows int
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "Columns:") || strings.Contains(line, "Spalten:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					cols, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
				}
			}
			if strings.Contains(line, "Lines:") || strings.Contains(line, "Zeilen:") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					rows, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
				}
			}
		}
		if cols > 0 && rows > 0 {
			// Estimate cell size based on common defaults
			// Windows Terminal typically uses 9x20 pixel cells
			return 9.0, 20.0, nil
		}
	}

	// Fallback to reasonable defaults for Windows terminals
	// These are typical values for Windows Terminal
	return 9.0, 20.0, nil
}
