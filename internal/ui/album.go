package ui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/juthrbog/spotui/internal/webapi"
)

type albumSavedMsg struct {
	uri   string
	name  string
	saved bool
	err   error
}

// albumOf returns the album that item names: the item itself for an album
// row, or the album of a track.
func albumOf(item *row) ref {
	switch {
	case isAlbumURI(item.uri):
		return ref{uri: item.uri, name: item.label}
	case isAlbumURI(item.album.uri):
		return item.album
	}
	return ref{}
}

// albumStates are the known save states of albums.
type albumStates struct {
	saved map[string]bool // from changes and from the saved albums
	// complete is true when the last load had every saved album, so an
	// album that is not in saved is not saved.
	complete bool
}

func (s albumStates) state(uri string) (saved, known bool) {
	if saved, known = s.saved[uri]; known {
		return saved, true
	}
	return false, s.complete
}

// loaded takes the saved albums from a full load. Albums that the user
// saved or removed in another app since the last load change state here.
func (s *albumStates) loaded(albums []webapi.Album) {
	s.saved = make(map[string]bool, len(albums))
	for _, a := range albums {
		s.saved[a.URI] = true
	}
	s.complete = len(albums) < webapi.MaxSavedAlbums
}

// albumLabel names the save action for the album with the URI uri.
func albumLabel(s albumStates, uri string) string {
	switch saved, known := s.state(uri); {
	case !known:
		return "Save or remove album"
	case saved:
		return "Remove album"
	}
	return "Save album"
}

// toggleAlbumOf saves the album of item to the library, or removes it when
// it is saved. If the state is not known yet, it asks the Web API first.
func (m *Model) toggleAlbumOf(item *row) tea.Cmd {
	a := albumOf(item)
	switch {
	case a.uri == "":
		return m.setFlash("No album is known for this item.", true)
	case !m.webReady():
		return m.setFlash("Saving albums needs the Web API. Push L to log in.", true)
	case !m.web.HasScope(webapi.ScopeLibraryModify):
		return m.setFlash("Saving albums needs a new permission. Push L to log in again.", true)
	case m.albumPending[a.uri]:
		return m.setFlash("Still changing your library for this album.", true)
	}
	if a.name == "" {
		a.name = "the album"
	}
	m.albumPending[a.uri] = true
	saved, known := m.albums.state(a.uri)
	ctx, web := m.ctx, m.web
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if !known {
			states, err := web.AlbumsSaved(ctx, []string{a.uri})
			if err != nil {
				return albumSavedMsg{uri: a.uri, name: a.name, err: err}
			}
			saved = states[a.uri]
		}
		err := web.SetAlbumsSaved(ctx, []string{a.uri}, !saved)
		return albumSavedMsg{uri: a.uri, name: a.name, saved: !saved, err: err}
	}
}

// albumSavedDone shows the result of a save or a remove and loads the saved
// albums again, so the Library tab shows the change.
func (m *Model) albumSavedDone(msg albumSavedMsg) tea.Cmd {
	delete(m.albumPending, msg.uri)
	switch {
	case errors.Is(msg.err, webapi.ErrMissingScope):
		return m.setFlash("Saving albums needs a new permission. Push L to log in again.", true)
	case errors.Is(msg.err, webapi.ErrNotLoggedIn):
		return m.setFlash("Saving albums needs the Web API. Push L to log in.", true)
	case msg.err != nil:
		return m.setFlash("Could not change your albums: "+msg.err.Error(), true)
	}
	m.albums.saved[msg.uri] = msg.saved
	flash := m.setFlash("Removed "+msg.name+" from your albums.", false)
	if msg.saved {
		flash = m.setFlash("Saved "+msg.name+" to your albums.", false)
	}
	return tea.Batch(flash, m.library.loadAlbums(m))
}
