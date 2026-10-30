package ui

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/juthrbog/spotui/internal/webapi"
)

// artistGroups are the sections of an artist page, in order.
var artistGroups = []struct{ group, title string }{
	{webapi.GroupAlbum, "Albums"},
	{webapi.GroupSingle, "Singles and EPs"},
	{webapi.GroupCompilation, "Compilations"},
	{webapi.GroupAppearsOn, "Appears On"},
}

// artistAlbums is one loaded section of an artist page.
type artistAlbums struct {
	albums []webapi.Album
	total  int
}

// artistPage shows the discography of an artist from the Web API. Its first
// rows play the artist or open all its tracks, which also works when the
// discography does not load.
type artistPage struct {
	uri    string
	name   string
	list   list
	filter filterBar

	groups map[string]*artistAlbums
	err    error
}

// openArtist opens the artist page, or the artist as a track list when the
// Web API is not ready.
func (m *Model) openArtist(uri, name string) tea.Cmd {
	if m.webReady() {
		return m.push(newArtistPage(uri, name))
	}
	p := newTracksPage(uri, name)
	p.hint = "Log in to the Web API with L to see the discography."
	if !m.web.Configured() {
		p.hint = "The discography needs a Spotify developer app. Open the Search tab (2) to set one up."
	}
	return m.push(p)
}

// reloadArtistPages runs after a Web API login. Artist pages that could not
// load their albums load them again. Artists that opened as a track list
// because there was no login become artist pages.
func (m *Model) reloadArtistPages() tea.Cmd {
	var cmds []tea.Cmd
	for t, s := range m.stacks {
		for i, p := range s {
			switch p := p.(type) {
			case *artistPage:
				if p.err != nil {
					p.err = nil
					p.groups = map[string]*artistAlbums{}
					p.rebuild()
					cmds = append(cmds, p.init(m))
				}
			case *tracksPage:
				if p.hint != "" {
					a := newArtistPage(p.uri, p.name)
					m.stacks[t][i] = a
					cmds = append(cmds, a.init(m))
				}
			}
		}
	}
	return tea.Batch(cmds...)
}

func newArtistPage(uri, name string) *artistPage {
	p := &artistPage{
		uri:    uri,
		name:   name,
		filter: newFilterBar(),
		groups: map[string]*artistAlbums{},
		list:   list{columns: []column{{weight: 5}, {weight: 3}, {width: 4}}},
	}
	p.rebuild()
	return p
}

func (p *artistPage) init(m *Model) tea.Cmd {
	cmds := make([]tea.Cmd, len(artistGroups))
	for i, g := range artistGroups {
		cmds[i] = loadArtistAlbums(m.ctx, m.web, p, g.group)
	}
	return tea.Batch(cmds...)
}

func (p *artistPage) title() string     { return p.name }
func (p *artistPage) typing() bool      { return p.filter.typing }
func (p *artistPage) stopTyping()       { p.filter.stop() }
func (p *artistPage) clearFilter() bool { return p.filter.clear(&p.list) }
func (p *artistPage) selection() *row   { return p.list.selected() }

func (p *artistPage) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case artistAlbumsMsg:
		if msg.page != p {
			return nil
		}
		if msg.err != nil {
			first := p.err == nil
			p.err = msg.err
			p.rebuild()
			switch {
			case errors.Is(msg.err, webapi.ErrNotLoggedIn):
				return m.startWebLogin()
			case first:
				return m.setFlash("Could not load the discography: "+msg.err.Error(), true)
			}
			return nil
		}
		p.groups[msg.group] = &artistAlbums{albums: msg.albums, total: msg.total}
		p.rebuild()
	case tea.KeyPressMsg:
		if p.filter.typing {
			return p.filter.update(&p.list, msg)
		}
		if p.list.handleKey(msg.String()) {
			return nil
		}
		switch msg.String() {
		case "f":
			return p.filter.start()
		case "enter":
			r := p.list.selected()
			switch {
			case r == nil:
			case r.play:
				return m.play(r.uri, "")
			default:
				return m.push(newTracksPage(r.uri, r.label))
			}
		}
	default:
		if p.filter.typing {
			return p.filter.update(&p.list, msg)
		}
	}
	return nil
}

func (p *artistPage) rebuild() {
	rows := []row{
		{cols: []string{"▶ Play " + p.name}, uri: p.uri, play: true, glyph: "▶", info: []string{p.name, "Play the artist"}},
		{cols: []string{"All tracks"}, uri: p.uri, label: p.name, glyph: "♪", info: []string{p.name, "All tracks of the artist"}},
	}
	if p.err != nil {
		rows = append(rows, note("Could not load the discography: "+p.err.Error()))
	}
	for _, g := range artistGroups {
		loaded := p.groups[g.group]
		switch {
		case loaded == nil && p.err != nil:
			continue
		case loaded != nil && len(loaded.albums) == 0:
			continue
		}
		rows = append(rows, row{kind: rowHeader}, row{kind: rowHeader, cols: []string{g.title}})
		if loaded == nil {
			rows = append(rows, note("Loading…"))
			continue
		}
		for _, a := range loaded.albums {
			rows = append(rows, albumRow(a))
		}
		if loaded.total > len(loaded.albums) {
			rows = append(rows, note(fmt.Sprintf("Showing the first %d of %d.", len(loaded.albums), loaded.total)))
		}
	}
	p.list.setRows(rows)
}

func (p *artistPage) view(m *Model, width, height int) string {
	if !p.filter.active() {
		return p.list.view(width, height, true, "")
	}
	return p.filter.view(&p.list, width) + "\n" + p.list.view(width, height-1, true, "")
}
