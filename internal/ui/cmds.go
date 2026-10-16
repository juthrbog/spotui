package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

type (
	engineEventMsg struct{ ev any }
	statusTickMsg  struct{}
	flashClearMsg  struct{ id int }
	actionDoneMsg  struct{ err error }

	statusMsg struct {
		st   *daemon.ApiStatus
		err  error
		poll bool // only polling replies schedule the next poll
	}
	playlistsMsg struct {
		items []daemon.ApiLibraryPlaylist
		err   error
	}
	albumsMsg struct {
		items []webapi.Album
		err   error
	}
	contextTracksMsg struct {
		page *tracksPage
		resp *daemon.ApiContextTracks
		err  error
	}
	searchMsg struct {
		seq int
		res *webapi.SearchResult
		err error
	}
	webLoginStartedMsg struct {
		login *webapi.Login
		err   error
	}
	webLoginDoneMsg struct{ err error }
)

func waitForEvent(eng *engine.Engine) tea.Cmd {
	return func() tea.Msg { return engineEventMsg{ev: <-eng.Events()} }
}

func fetchStatus(ctx context.Context, eng *engine.Engine, poll bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		st, err := eng.Status(ctx)
		return statusMsg{st: st, err: err, poll: poll}
	}
}

func statusTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return statusTickMsg{} })
}

func loadPlaylists(ctx context.Context, eng *engine.Engine) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		items, err := eng.LibraryPlaylists(ctx)
		return playlistsMsg{items: items, err: err}
	}
}

func loadAlbums(ctx context.Context, web *webapi.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		items, err := web.SavedAlbums(ctx)
		return albumsMsg{items: items, err: err}
	}
}

func fetchContextTracks(ctx context.Context, eng *engine.Engine, page *tracksPage, delay time.Duration) tea.Cmd {
	return func() tea.Msg {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil
			}
		}
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		resp, err := eng.ContextTracks(ctx, page.uri)
		return contextTracksMsg{page: page, resp: resp, err: err}
	}
}

func search(ctx context.Context, web *webapi.Client, query string, seq int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		res, err := web.Search(ctx, query)
		return searchMsg{seq: seq, res: res, err: err}
	}
}
