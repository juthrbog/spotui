package ui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

// libraryPage lists Liked Songs, the user's playlists, and saved albums.
type libraryPage struct {
	list list

	playlists        []daemon.ApiLibraryPlaylist
	playlistsErr     error
	playlistsLoading bool

	albums        []webapi.Album
	albumsErr     error
	albumsLoading bool
}

func newLibraryPage() *libraryPage {
	return &libraryPage{list: list{columns: []column{{weight: 3}, {weight: 2}, {width: 11, right: true}}}}
}

func (p *libraryPage) title() string   { return "Library" }
func (p *libraryPage) typing() bool    { return false }
func (p *libraryPage) selection() *row { return p.list.selected() }

// load fetches playlists and, when the Web API is set up, saved albums.
func (p *libraryPage) load(m *Model) tea.Cmd {
	p.playlistsLoading = true
	p.playlistsErr = nil
	cmd := tea.Batch(loadPlaylists(m.ctx, m.eng), p.loadAlbums(m))
	p.rebuild(m)
	return cmd
}

func (p *libraryPage) loadAlbums(m *Model) tea.Cmd {
	defer p.rebuild(m)
	if !m.web.Configured() || !m.web.LoggedIn() {
		return nil
	}
	p.albumsLoading = true
	p.albumsErr = nil
	return loadAlbums(m.ctx, m.web)
}

func (p *libraryPage) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case playlistsMsg:
		p.playlistsLoading = false
		p.playlists, p.playlistsErr = msg.items, msg.err
		p.rebuild(m)
	case albumsMsg:
		p.albumsLoading = false
		p.albums, p.albumsErr = msg.items, msg.err
		p.rebuild(m)
		if errors.Is(msg.err, webapi.ErrNotLoggedIn) {
			return m.startWebLogin()
		}
	case tea.KeyPressMsg:
		if p.list.handleKey(msg.String()) {
			return nil
		}
		if msg.String() == "enter" {
			if r := p.list.selected(); r != nil {
				return m.push(newTracksPage(r.uri, r.label))
			}
		}
	}
	return nil
}

func (p *libraryPage) rebuild(m *Model) {
	username := m.username()
	rows := []row{{kind: rowHeader, cols: []string{"Playlists"}}}

	if username != "" {
		rows = append(rows, row{
			cols:  []string{"♥ Liked Songs"},
			uri:   engine.LikedSongsURI(username),
			label: "Liked Songs",
			glyph: "♥",
			info:  []string{"Liked Songs", "Playlist"},
		})
	}
	switch {
	case p.playlistsLoading && len(p.playlists) == 0:
		rows = append(rows, note("Loading playlists…"))
	case p.playlistsErr != nil:
		rows = append(rows, note("Could not load playlists: "+p.playlistsErr.Error()+" (press R to retry)"))
	case m.ready && len(p.playlists) == 0:
		rows = append(rows, note("No playlists."))
	}
	for _, pl := range p.playlists {
		name := pl.Name
		if len(pl.Folder) > 0 {
			name = strings.Join(pl.Folder, " / ") + " / " + name
		}
		owner, kind := "", "Playlist"
		if pl.OwnerUsername != "" && pl.OwnerUsername != username {
			owner = "by " + pl.OwnerUsername
			kind += " · " + owner
		}
		count := plural(int(pl.Length), "track")
		rows = append(rows, row{
			cols:  []string{name, owner, count},
			uri:   pl.Uri,
			label: pl.Name,
			image: deref(pl.ImageUrl),
			info:  []string{pl.Name, kind, count},
		})
	}

	rows = append(rows, row{kind: rowHeader}, row{kind: rowHeader, cols: []string{"Albums"}})
	switch {
	case !m.web.Configured():
		rows = append(rows, note("Saved albums need a Spotify developer app. Open the Search tab (2) to set one up."))
	case !m.web.LoggedIn():
		rows = append(rows, note("Log in to the Web API to see saved albums (press L)."))
	case p.albumsLoading && len(p.albums) == 0:
		rows = append(rows, note("Loading albums…"))
	case p.albumsErr != nil:
		rows = append(rows, note("Could not load albums: "+p.albumsErr.Error()+" (press R to retry)"))
	case len(p.albums) == 0:
		rows = append(rows, note("No saved albums."))
	}
	for _, a := range p.albums {
		artists := webapi.ArtistNames(a.Artists)
		rows = append(rows, row{
			cols:  []string{a.Name, artists, a.Year()},
			uri:   a.URI,
			label: a.Name,
			image: webapi.PickImage(a.Images),
			info:  []string{a.Name, artists, albumKind(a)},
		})
	}
	p.list.setRows(rows)
}

func (p *libraryPage) view(m *Model, width, height int) string {
	return p.list.view(width, height, true, "")
}

func note(text string) row { return row{kind: rowNote, cols: []string{text}} }

func albumKind(a webapi.Album) string {
	if y := a.Year(); y != "" {
		return "Album · " + y
	}
	return "Album"
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
