package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/webapi"
)

// ref names a Spotify item that a row links to.
type ref struct {
	uri  string
	name string
}

// refs pairs URIs with their names. go-librespot gives both lists in the
// same order.
func refs(uris, names []string) []ref {
	out := make([]ref, 0, len(uris))
	for i, uri := range uris {
		if uri == "" {
			continue
		}
		name := uri
		if i < len(names) && names[i] != "" {
			name = names[i]
		}
		out = append(out, ref{uri: uri, name: name})
	}
	return out
}

// artistRefs returns links to the artists of a Web API track.
func artistRefs(artists []webapi.Artist) []ref {
	out := make([]ref, 0, len(artists))
	for _, a := range artists {
		if a.URI != "" {
			out = append(out, ref{uri: a.URI, name: a.Name})
		}
	}
	return out
}

func isTrackURI(uri string) bool { return strings.HasPrefix(uri, "spotify:track:") }
func isAlbumURI(uri string) bool { return strings.HasPrefix(uri, "spotify:album:") }

// trackRow returns a row with the links of t.
func trackRow(t *daemon.ApiTrack) row {
	return row{
		uri:     t.Uri,
		label:   t.Name,
		album:   ref{uri: t.AlbumUri, name: t.AlbumName},
		artists: refs(t.ArtistUris, t.ArtistNames),
	}
}

// actionItem returns the item that an action applies to: the selected track
// or album, or the track that plays when no track or album is selected. In
// a track list it is always the selected row, also for an episode or a
// local file, so an action never applies to a track that the user did not
// pick. It returns nil when there is no such item.
func (m *Model) actionItem() *row {
	sel := m.current().selection()
	if _, ok := m.current().(*tracksPage); ok || (sel != nil && (isTrackURI(sel.uri) || isAlbumURI(sel.uri))) {
		return sel
	}
	if m.status == nil || m.status.Track == nil {
		return nil
	}
	r := trackRow(m.status.Track)
	return &r
}

func (m *Model) goToAlbum() tea.Cmd  { return m.goToAlbumOf(m.actionItem()) }
func (m *Model) goToArtist() tea.Cmd { return m.goToArtistOf(m.actionItem()) }

func (m *Model) goToAlbumOf(t *row) tea.Cmd {
	switch {
	case t == nil:
		return m.setFlash("Select a track or play one first.", true)
	case t.album.uri == "":
		return m.setFlash("No album is known for this item.", true)
	}
	return m.push(newTracksPage(t.album.uri, t.album.name))
}

func (m *Model) goToArtistOf(t *row) tea.Cmd {
	switch {
	case t == nil:
		return m.setFlash("Select a track or play one first.", true)
	case len(t.artists) == 0:
		return m.setFlash("No artist is known for this item.", true)
	case len(t.artists) > 1:
		return m.push(newArtistsPage(t.label, t.artists))
	}
	a := t.artists[0]
	if p, ok := m.current().(*artistPage); ok && p.uri == a.uri {
		return m.setFlash("This is the page of "+a.name+".", false)
	}
	return m.openArtist(a.uri, a.name)
}

// goToContext opens the playlist, album, or artist that plays. If a page
// of it is already open, it goes back to that page.
func (m *Model) goToContext() tea.Cmd {
	uri := ""
	if m.status != nil {
		uri = deref(m.status.ContextUri)
	}
	if uri == "" {
		return m.setFlash("Nothing plays.", true)
	}
	for t, s := range m.stacks {
		for i, p := range s {
			if u := contextOf(p); u != "" && sameContext(u, uri) {
				m.current().stopTyping()
				m.tab = tab(t)
				m.stacks[t] = s[:i+1]
				return nil
			}
		}
	}
	name := deref(m.status.ContextName)
	if name == "" {
		name = contextKind(uri)
	}
	if strings.HasPrefix(uri, "spotify:artist:") {
		return m.openArtist(uri, name)
	}
	return m.push(newTracksPage(uri, name))
}

// contextOf returns the context URI that a page shows, or an empty string.
func contextOf(p page) string {
	switch p := p.(type) {
	case *tracksPage:
		return p.uri
	case *artistPage:
		return p.uri
	}
	return ""
}

// isLikedSongs reports whether uri is one of the two context URIs that
// go-librespot accepts for Liked Songs.
func isLikedSongs(uri string) bool {
	return uri == "spotify:collection:tracks" ||
		strings.HasPrefix(uri, "spotify:user:") && strings.HasSuffix(uri, ":collection")
}

func sameContext(a, b string) bool {
	return a == b || isLikedSongs(a) && isLikedSongs(b)
}

// contextKind names the type of a context, for a context without a name.
func contextKind(uri string) string {
	switch {
	case isLikedSongs(uri):
		return "Liked Songs"
	case strings.HasPrefix(uri, "spotify:album:"):
		return "Album"
	case strings.HasPrefix(uri, "spotify:artist:"):
		return "Artist"
	case strings.HasPrefix(uri, "spotify:show:"):
		return "Show"
	}
	return "Playlist"
}

// artistsPage asks which artist of a track to open.
type artistsPage struct {
	track string
	list  list
}

func newArtistsPage(track string, artists []ref) *artistsPage {
	p := &artistsPage{track: track, list: list{columns: []column{{weight: 1}}}}
	rows := []row{{kind: rowHeader, cols: []string{"Which artist?"}}}
	for _, a := range artists {
		rows = append(rows, row{cols: []string{a.name}, uri: a.uri, label: a.name, glyph: "♪", info: []string{a.name, "Artist"}})
	}
	p.list.setRows(rows)
	return p
}

func (p *artistsPage) title() string   { return "Artists of " + p.track }
func (p *artistsPage) typing() bool    { return false }
func (p *artistsPage) stopTyping()     {}
func (p *artistsPage) selection() *row { return p.list.selected() }
func (p *artistsPage) view(_ *Model, width, height int) string {
	return p.list.view(width, height, true, "")
}

func (p *artistsPage) update(m *Model, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok || p.list.handleKey(key.String()) {
		return nil
	}
	if key.String() == "enter" {
		if r := p.list.selected(); r != nil {
			return m.openArtist(r.uri, r.label)
		}
	}
	return nil
}
