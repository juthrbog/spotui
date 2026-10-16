package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	colorAccent = lipgloss.Color("#1DB954")

	styleAccent     = lipgloss.NewStyle().Foreground(colorAccent)
	styleAccentBold = styleAccent.Bold(true)
	styleBold       = lipgloss.NewStyle().Bold(true)
	styleDim        = lipgloss.NewStyle().Faint(true)
	styleError      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleSelected   = lipgloss.NewStyle().Background(colorAccent).Foreground(lipgloss.Color("#000000")).Bold(true)
	styleTabActive  = styleAccentBold.Underline(true)
)

// fit truncates s to w cells and pads it with spaces to exactly w cells.
func fit(s string, w int, alignRight bool) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	pad := strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
	if alignRight {
		return pad + s
	}
	return s + pad
}

// fitBlock makes s exactly h lines of at most w cells, so the frame never
// wraps or jumps.
func fitBlock(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "")
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// spread puts left and right on one line of width w, with space between.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, max(w-lipgloss.Width(right)-1, 0), "…") + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// formatMs formats milliseconds as m:ss or h:mm:ss.
func formatMs(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// link renders url as a clickable, wrapped link.
func link(url string, w int) string {
	return styleAccent.Hyperlink(url).Render(ansi.Hardwrap(url, max(w, 20), false))
}

// openBrowser opens url in the default browser. It fails quietly, because the
// UI always shows the URL too. Tests replace it.
var openBrowser = func(url string) {
	name, args := browserCommand(runtime.GOOS, url)
	cmd := exec.Command(name, args...) // #nosec G204 -- fixed program, URL from Spotify login flow
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}

// browserCommand returns the program that opens url on goos. On Windows,
// rundll32 is used instead of "cmd /c start", because cmd splits the URL at
// each "&".
func browserCommand(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
