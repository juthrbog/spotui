package ui

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// filterBar is a text box that narrows the list of a page. It works on the
// rows that are already loaded, so it makes no network calls.
type filterBar struct {
	input  textinput.Model
	typing bool
}

const filterInputWidth = 30

func newFilterBar() filterBar {
	in := textinput.New()
	in.Prompt = "Filter: "
	in.Placeholder = "type to narrow this list"
	in.CharLimit = 100
	in.SetWidth(filterInputWidth)
	return filterBar{input: in}
}

// active reports whether the bar is on screen: while typing, or while a
// filter is set.
func (f *filterBar) active() bool { return f.typing || f.input.Value() != "" }

func (f *filterBar) start() tea.Cmd {
	f.typing = true
	return f.input.Focus()
}

// stop ends typing and keeps the filter.
func (f *filterBar) stop() {
	f.typing = false
	f.input.Blur()
}

// clear ends typing and removes the filter. It reports whether there was
// anything to clear.
func (f *filterBar) clear(l *list) bool {
	if !f.active() {
		return false
	}
	f.stop()
	f.input.Reset()
	l.setFilter("")
	return true
}

// update handles a message while typing. Enter keeps the filter, esc removes
// it, and up and down move the selection without leaving the box.
func (f *filterBar) update(l *list, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			f.stop()
			if f.input.Value() == "" {
				l.setFilter("")
			}
			return nil
		case "esc":
			f.clear(l)
			return nil
		case "up", "down", "pgup", "pgdown":
			l.handleKey(k.String())
			return nil
		}
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	l.setFilter(f.input.Value())
	return cmd
}

func (f *filterBar) view(l *list, width int) string {
	shown, total := l.counts()
	line := f.input.View() + styleDim.Render(fmt.Sprintf("  %d of %d", shown, total))
	if !f.typing {
		line += styleDim.Render("  ·  f edit  ·  esc clear")
	}
	return ansi.Truncate(line, width, "…")
}
