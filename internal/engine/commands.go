package engine

import (
	"context"
	"fmt"

	"github.com/devgianlu/go-librespot/daemon"
)

// Status returns the current playback state.
func (e *Engine) Status(ctx context.Context) (*daemon.ApiStatus, error) {
	resp, err := e.call(ctx, daemon.ApiRequestTypeStatus, nil)
	if err != nil {
		return nil, err
	}
	st, ok := resp.(*daemon.ApiStatus)
	if !ok {
		return nil, fmt.Errorf("unexpected status reply %T", resp)
	}
	return st, nil
}

// Play plays a track, album, playlist, or artist URI. If skipTo is set, the
// context starts at that track.
func (e *Engine) Play(ctx context.Context, uri, skipTo string) error {
	_, err := e.call(ctx, daemon.ApiRequestTypePlay, daemon.ApiPlay{Uri: uri, SkipToUri: skipTo})
	return err
}

// PlayPause toggles between playing and paused.
func (e *Engine) PlayPause(ctx context.Context) error {
	_, err := e.call(ctx, daemon.ApiRequestTypePlayPause, nil)
	return err
}

// Next skips to the next track.
func (e *Engine) Next(ctx context.Context) error {
	_, err := e.call(ctx, daemon.ApiRequestTypeNext, daemon.ApiNext{})
	return err
}

// Prev goes to the previous track, or to the start of the current track if
// it has played for more than a few seconds.
func (e *Engine) Prev(ctx context.Context) error {
	_, err := e.call(ctx, daemon.ApiRequestTypePrev, nil)
	return err
}

// Seek moves the playback position by deltaMs milliseconds.
func (e *Engine) Seek(ctx context.Context, deltaMs int64) error {
	_, err := e.call(ctx, daemon.ApiRequestTypeSeek, daemon.ApiSeek{Position: deltaMs, Relative: true})
	return err
}

// AdjustVolume changes the volume by delta percent.
func (e *Engine) AdjustVolume(ctx context.Context, delta int32) error {
	_, err := e.call(ctx, daemon.ApiRequestTypeSetVolume, daemon.ApiSetVolume{Volume: delta, Relative: true})
	return err
}

// SetShuffle turns shuffle on or off for the current context.
func (e *Engine) SetShuffle(ctx context.Context, on bool) error {
	_, err := e.call(ctx, daemon.ApiRequestTypeSetShufflingContext, on)
	return err
}

// RepeatMode is the repeat setting.
type RepeatMode int

const (
	RepeatOff RepeatMode = iota
	RepeatContext
	RepeatTrack
)

// SetRepeat sets the repeat mode.
func (e *Engine) SetRepeat(ctx context.Context, mode RepeatMode) error {
	if _, err := e.call(ctx, daemon.ApiRequestTypeSetRepeatingContext, mode != RepeatOff); err != nil {
		return err
	}
	_, err := e.call(ctx, daemon.ApiRequestTypeSetRepeatingTrack, mode == RepeatTrack)
	return err
}

// LibraryPlaylists returns the playlists in the user's library, including
// playlists the user follows but does not own.
func (e *Engine) LibraryPlaylists(ctx context.Context) ([]daemon.ApiLibraryPlaylist, error) {
	const pageSize = 200
	var all []daemon.ApiLibraryPlaylist
	for {
		resp, err := e.call(ctx, daemon.ApiRequestTypeLibraryPlaylists,
			daemon.ApiRequestDataLibraryPlaylists{Offset: len(all), Limit: pageSize})
		if err != nil {
			return nil, err
		}
		page, ok := resp.(*daemon.ApiLibraryPlaylists)
		if !ok {
			return nil, fmt.Errorf("unexpected playlists reply %T", resp)
		}
		all = append(all, page.Items...)
		if len(page.Items) == 0 || len(all) >= page.Total {
			return all, nil
		}
	}
}

// ContextTracks lists the tracks of a playlist, album, artist, or Liked
// Songs URI. The list fills in over several calls: call again while Ready is
// false or Cached is less than Length.
func (e *Engine) ContextTracks(ctx context.Context, uri string) (*daemon.ApiContextTracks, error) {
	resp, err := e.call(ctx, daemon.ApiRequestTypeContextTracks, daemon.ApiRequestDataContextTracks{Uri: uri})
	if err != nil {
		return nil, err
	}
	list, ok := resp.(*daemon.ApiContextTracks)
	if !ok {
		return nil, fmt.Errorf("unexpected tracks reply %T", resp)
	}
	return list, nil
}

// LikedSongsURI is the context URI of the user's Liked Songs.
func LikedSongsURI(username string) string {
	return "spotify:user:" + username + ":collection"
}
