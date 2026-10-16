package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// column describes one column of a list. A column with weight 0 has a fixed
// width; the others share the remaining space by weight.
type column struct {
	weight int
	width  int
	right  bool
}

type rowKind int

const (
	rowItem   rowKind = iota
	rowHeader         // section title, not selectable
	rowNote           // information line, not selectable
)

// row is one line of a list.
type row struct {
	kind  rowKind
	cols  []string
	uri   string // Spotify URI of the item
	label string // title for a page opened from this row
	play  bool   // enter plays the URI instead of opening it

	// Shown in the side panel.
	image string   // cover art URL
	glyph string   // symbol shown when there is no image
	info  []string // title, then detail lines
}

func (r row) selectable() bool { return r.kind == rowItem }

// list is a scrolling, selectable list of rows.
type list struct {
	columns []column
	rows    []row
	cursor  int
	offset  int
	height  int // height of the last render, used for page up and down
}

func (l *list) setRows(rows []row) {
	l.rows = rows
	l.cursor = min(l.cursor, max(len(rows)-1, 0))
	l.move(0)
}

func (l *list) selected() *row {
	if l.cursor < 0 || l.cursor >= len(l.rows) || !l.rows[l.cursor].selectable() {
		return nil
	}
	return &l.rows[l.cursor]
}

// move moves the cursor by delta, skipping rows that cannot be selected.
func (l *list) move(delta int) {
	n := len(l.rows)
	if n == 0 {
		l.cursor = 0
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	start := min(max(l.cursor+delta, 0), n-1)
	for i := start; i >= 0 && i < n; i += dir {
		if l.rows[i].selectable() {
			l.cursor = i
			return
		}
	}
	for i := start; i >= 0 && i < n; i -= dir {
		if l.rows[i].selectable() {
			l.cursor = i
			return
		}
	}
}

// handleKey moves the cursor for navigation keys and reports whether the key
// was used.
func (l *list) handleKey(key string) bool {
	page := max(l.height-1, 1)
	switch key {
	case "up", "k":
		l.move(-1)
	case "down", "j":
		l.move(1)
	case "pgup", "ctrl+u":
		l.move(-page)
	case "pgdown", "ctrl+d":
		l.move(page)
	case "home", "g":
		l.cursor = 0
		l.move(0)
	case "end", "G":
		l.cursor = len(l.rows) - 1
		l.move(0)
	default:
		return false
	}
	return true
}

func layoutColumns(cols []column, total int) []int {
	const gap = 2
	avail := total - gap*(len(cols)-1)
	fixed, weights := 0, 0
	for _, c := range cols {
		if c.weight == 0 {
			fixed += c.width
		} else {
			weights += c.weight
		}
	}
	flex := max(avail-fixed, 0)
	widths := make([]int, len(cols))
	used, last := 0, -1
	for i, c := range cols {
		if c.weight == 0 {
			widths[i] = c.width
			continue
		}
		widths[i] = flex * c.weight / weights
		used += widths[i]
		last = i
	}
	if last >= 0 {
		widths[last] += flex - used
	}
	return widths
}

// view renders the list. focused controls the selection highlight, and the
// row whose URI is playingURI gets a play marker.
func (l *list) view(width, height int, focused bool, playingURI string) string {
	l.height = height
	if height <= 0 {
		return ""
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+height {
		l.offset = l.cursor - height + 1
	}
	l.offset = min(max(l.offset, 0), max(len(l.rows)-height, 0))
	// Keep the section title above the first item visible.
	if l.offset == 1 && l.cursor < height {
		l.offset = 0
	}

	const prefixWidth = 2
	widths := layoutColumns(l.columns, width-prefixWidth)

	var b strings.Builder
	end := min(len(l.rows), l.offset+height)
	for i := l.offset; i < end; i++ {
		if i > l.offset {
			b.WriteByte('\n')
		}
		r := l.rows[i]
		switch r.kind {
		case rowHeader:
			b.WriteString(styleAccentBold.Render(ansi.Truncate(strings.Join(r.cols, " "), width, "…")))
			continue
		case rowNote:
			b.WriteString(styleDim.Render(ansi.Truncate("  "+strings.Join(r.cols, " "), width, "…")))
			continue
		}

		selected := i == l.cursor
		playing := playingURI != "" && r.uri == playingURI
		prefix := "  "
		switch {
		case playing:
			prefix = "▶ "
		case selected && !focused:
			prefix = "› "
		}

		cells := make([]string, len(l.columns))
		for c := range l.columns {
			text := ""
			if c < len(r.cols) {
				text = r.cols[c]
			}
			cells[c] = fit(text, widths[c], l.columns[c].right)
		}

		switch {
		case selected && focused:
			b.WriteString(styleSelected.Render(fit(prefix+strings.Join(cells, "  "), width, false)))
		case playing:
			b.WriteString(styleAccent.Render(prefix + strings.Join(cells, "  ")))
		default:
			b.WriteString(prefix + cells[0])
			if len(cells) > 1 {
				b.WriteString("  " + styleDim.Render(strings.Join(cells[1:], "  ")))
			}
		}
	}
	return b.String()
}
