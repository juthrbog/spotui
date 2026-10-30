package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// action is one line of the action menu.
type action struct {
	label    string
	key      string // direct key outside the menu, or empty
	needsWeb bool   // needs the Web API login
	run      func(m *Model, item *row) tea.Cmd
}

// actionMenu is the popup that a opens. It lists the actions for one item.
// It keeps a copy of the item, so an action applies to that item even when
// the selection or the playing track changes while the menu is open.
type actionMenu struct {
	title  string
	item   row
	items  []action
	cursor int
}

// actionsFor returns the actions that apply to item.
func actionsFor(item *row) []action {
	var out []action
	if isAlbumURI(item.uri) {
		out = append(out, action{label: "Play", run: func(m *Model, item *row) tea.Cmd { return m.play(item.uri, "") }})
	} else if item.album.uri != "" {
		out = append(out, action{label: "Go to album", key: "A", run: (*Model).goToAlbumOf})
	}
	if len(item.artists) > 0 {
		out = append(out, action{label: "Go to artist", key: "I", run: (*Model).goToArtistOf})
	}
	return out
}

func (m *Model) openMenu() tea.Cmd {
	item := m.actionItem()
	if item == nil {
		return m.setFlash("Select a track or play one first.", true)
	}
	items := actionsFor(item)
	if len(items) == 0 {
		return m.setFlash("No actions for this item.", true)
	}
	title := item.label
	if title == "" && len(item.info) > 0 {
		title = item.info[0]
	}
	if sel := m.current().selection(); sel == nil || sel.uri != item.uri {
		title = "Now playing: " + title
	}
	m.menu = &actionMenu{title: title, item: *item, items: items}
	return nil
}

// menuKey handles a key while the menu is open.
func (m *Model) menuKey(key string) tea.Cmd {
	menu := m.menu
	switch key {
	case "up", "k":
		menu.cursor = max(menu.cursor-1, 0)
	case "down", "j":
		menu.cursor = min(menu.cursor+1, len(menu.items)-1)
	case "enter":
		return m.runAction(menu.items[menu.cursor])
	case "esc", "a", "q":
		m.menu = nil
	default:
		for _, it := range menu.items {
			if it.key != "" && it.key == key {
				return m.runAction(it)
			}
		}
	}
	return nil
}

// runAction closes the menu and runs it on the item of the menu.
func (m *Model) runAction(it action) tea.Cmd {
	item := m.menu.item
	m.menu = nil
	if it.needsWeb && !m.webReady() {
		return m.setFlash(it.label+" needs the Web API. Push L to log in.", true)
	}
	return it.run(m, &item)
}

// webReady reports whether the Web API is set up and logged in.
func (m *Model) webReady() bool { return m.web.Configured() && m.web.LoggedIn() }

// view renders the menu as a box that fits in width cells.
func (menu *actionMenu) view(m *Model, width int) string {
	const keyWidth = 5
	webReady := m.webReady()
	labels := make([]string, len(menu.items))
	inner := max(ansi.StringWidth(menu.title), 24)
	for i, it := range menu.items {
		labels[i] = it.label
		if it.needsWeb && !webReady {
			labels[i] += " (needs L login)"
		}
		inner = max(inner, ansi.StringWidth(labels[i])+keyWidth+2)
	}
	inner = min(inner, max(width-4, 10))
	labelWidth := inner - keyWidth - 2

	lines := []string{styleBold.Render(fit(menu.title, inner, false)), ""}
	for i, it := range menu.items {
		label := labels[i]
		line := "  " + fit(label, labelWidth, false) + fit(it.key, keyWidth, true)
		switch {
		case i == menu.cursor:
			line = styleSelected.Render(fit("› "+label, labelWidth+2, false) + fit(it.key, keyWidth, true))
		case it.needsWeb && !webReady:
			line = styleDim.Render(line)
		}
		lines = append(lines, line)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

// overlayMenu fits the list area to width by height cells and draws the
// menu over it when the menu is open.
func (m *Model) overlayMenu(base string, width, height int) string {
	base = fitBlock(base, width, height)
	if m.menu == nil {
		return base
	}
	box := m.menu.view(m, width)
	x := max((width-lipgloss.Width(box))/2, 0)
	y := max(min(2, height-lipgloss.Height(box)), 0)
	return fitBlock(lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render(), width, height)
}
