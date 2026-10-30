package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"
)

const (
	tracksPollInterval = 700 * time.Millisecond
	// tracksMaxStalls stops polling when the list has not grown for this many
	// polls in a row.
	tracksMaxStalls = 40
)

// tracksPage lists the tracks of a playlist, album, artist, or Liked Songs.
// go-librespot fills in the list in the background, so the page polls until
// every track has its details.
type tracksPage struct {
	uri    string
	name   string
	hint   string // line shown above the list
	list   list
	filter filterBar

	resp       *daemon.ApiContextTracks
	err        error
	lastCached int
	stalls     int
}

func newTracksPage(uri, name string) *tracksPage {
	return &tracksPage{
		uri:    uri,
		name:   name,
		filter: newFilterBar(),
		list: list{columns: []column{
			{width: 4, right: true},
			{weight: 4},
			{weight: 3},
			{weight: 3},
			{width: 7, right: true},
		}},
	}
}

func (p *tracksPage) init(m *Model) tea.Cmd { return fetchContextTracks(m.ctx, m.eng, p, 0) }
func (p *tracksPage) typing() bool          { return p.filter.typing }
func (p *tracksPage) stopTyping()           { p.filter.stop() }
func (p *tracksPage) clearFilter() bool     { return p.filter.clear(&p.list) }
func (p *tracksPage) selection() *row       { return p.list.selected() }

func (p *tracksPage) title() string {
	if p.resp == nil || !p.resp.Ready {
		return p.name
	}
	t := p.name + " · " + plural(p.resp.Length, "track")
	if p.resp.Cached < p.resp.Length && p.stalls < tracksMaxStalls {
		t += fmt.Sprintf(" (loading %d/%d)", p.resp.Cached, p.resp.Length)
	}
	return t
}

func (p *tracksPage) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case contextTracksMsg:
		if msg.page != p {
			return nil
		}
		if msg.err != nil {
			p.err = msg.err
			return nil
		}
		p.err = nil
		p.resp = msg.resp
		p.rebuild()
		if msg.resp.Ready && msg.resp.Cached >= msg.resp.Length {
			return nil
		}
		if msg.resp.Ready && msg.resp.Cached == p.lastCached {
			p.stalls++
		} else {
			p.stalls = 0
		}
		p.lastCached = msg.resp.Cached
		if p.stalls >= tracksMaxStalls {
			return nil
		}
		return fetchContextTracks(m.ctx, m.eng, p, tracksPollInterval)
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
			if r := p.list.selected(); r != nil {
				return m.play(p.uri, r.uri)
			}
		}
	default:
		if p.filter.typing {
			return p.filter.update(&p.list, msg)
		}
	}
	return nil
}

func (p *tracksPage) rebuild() {
	rows := make([]row, len(p.resp.Tracks))
	for i, it := range p.resp.Tracks {
		num := strconv.Itoa(i + 1)
		if it.Track == nil {
			rows[i] = row{cols: []string{num, "…"}, uri: it.Uri}
			continue
		}
		t := it.Track
		artists := strings.Join(t.ArtistNames, ", ")
		r := trackRow(t)
		r.cols = []string{num, t.Name, artists, t.AlbumName, formatMs(int64(t.Duration))}
		r.uri = it.Uri
		r.image = deref(t.AlbumCoverUrl)
		r.info = []string{t.Name, artists, t.AlbumName}
		rows[i] = r
	}
	p.list.setRows(rows)
}

func (p *tracksPage) view(m *Model, width, height int) string {
	head := ""
	if p.hint != "" {
		head = styleDim.Render(fit(p.hint, width, false)) + "\n"
		height--
	}
	if p.filter.active() {
		head += p.filter.view(&p.list, width) + "\n"
		height--
	}
	switch {
	case p.err != nil:
		return head + styleError.Render("Could not load tracks: "+p.err.Error())
	case p.resp == nil || !p.resp.Ready:
		return head + styleDim.Render("Loading tracks…")
	case p.resp.Length == 0:
		return head + styleDim.Render("No tracks.")
	}
	return head + p.list.view(width, height, true, m.playingURI())
}
