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

	// gen counts the loads of the list. A reload starts a new load, and the
	// replies of the old one are dropped, so only one poll runs at a time.
	gen       int
	reloading bool // keep the old rows until the new list is ready
}

func newTracksPage(uri, name string) *tracksPage {
	return &tracksPage{
		uri:    uri,
		name:   name,
		filter: newFilterBar(),
		list: list{columns: []column{
			{width: 4, right: true},
			{width: 1}, // ♥ for liked tracks
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
		if msg.page != p || msg.gen != p.gen {
			return nil
		}
		if msg.err != nil {
			p.err = msg.err
			return nil
		}
		if p.reloading && !msg.resp.Ready {
			return fetchContextTracks(m.ctx, m.eng, p, tracksPollInterval)
		}
		p.reloading = false
		p.err = nil
		p.resp = msg.resp
		liked := p.loadLiked(m)
		p.rebuild(m)
		return tea.Batch(liked, p.poll(m))
	case likedStatesMsg:
		if p.resp != nil {
			p.rebuild(m)
		}
	case likedSetMsg:
		switch {
		case msg.err != nil:
		case isLikedSongs(p.uri):
			// go-librespot dropped its copy of Liked Songs, so this loads
			// the new list.
			return p.reload(m)
		case p.resp != nil:
			p.rebuild(m)
		}
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

// reload loads the list again. The old rows stay on screen, with the
// cursor, until the new list is ready.
func (p *tracksPage) reload(m *Model) tea.Cmd {
	p.gen++
	p.stalls, p.lastCached = 0, 0
	p.reloading = p.resp != nil
	return fetchContextTracks(m.ctx, m.eng, p, 0)
}

// poll asks for the list again until every track has its details.
func (p *tracksPage) poll(m *Model) tea.Cmd {
	resp := p.resp
	if resp.Ready && resp.Cached >= resp.Length {
		return nil
	}
	if resp.Ready && resp.Cached == p.lastCached {
		p.stalls++
	} else {
		p.stalls = 0
	}
	p.lastCached = resp.Cached
	if p.stalls >= tracksMaxStalls {
		return nil
	}
	return fetchContextTracks(m.ctx, m.eng, p, tracksPollInterval)
}

// loadLiked loads whether the tracks of the list are liked. Every track in
// Liked Songs is liked, so that list needs no request.
func (p *tracksPage) loadLiked(m *Model) tea.Cmd {
	uris := make([]string, len(p.resp.Tracks))
	for i, it := range p.resp.Tracks {
		uris[i] = it.Uri
	}
	if !isLikedSongs(p.uri) {
		return m.wantLiked(uris)
	}
	for _, uri := range uris {
		m.liked[uri], m.likedAsked[uri] = true, true
	}
	return nil
}

func (p *tracksPage) rebuild(m *Model) {
	// In Liked Songs every track is liked, so the hearts would only add noise.
	hearts := !isLikedSongs(p.uri)
	rows := make([]row, len(p.resp.Tracks))
	for i, it := range p.resp.Tracks {
		num := strconv.Itoa(i + 1)
		heart := ""
		if hearts && m.liked[it.Uri] {
			heart = "♥"
		}
		if it.Track == nil {
			rows[i] = row{cols: []string{num, heart, "…"}, uri: it.Uri}
			continue
		}
		t := it.Track
		artists := strings.Join(t.ArtistNames, ", ")
		r := trackRow(t)
		r.cols = []string{num, heart, t.Name, artists, t.AlbumName, formatMs(int64(t.Duration))}
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
