package engine

import (
	"context"
	"fmt"
	"slices"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/daemon"
)

// The HTTP server of go-librespot checks the data of library requests, but
// requests over the channel skip those checks, and some bad data makes the
// player panic. The wrappers in this file make the same checks.

// maxLibraryURIs is the most URIs that go-librespot takes in one library
// request.
const maxLibraryURIs = 50

// canonicalURIs checks that uris holds 1 to 50 URIs of the given types and
// returns them in canonical form.
func canonicalURIs(uris []string, types ...librespot.SpotifyIdType) ([]string, error) {
	if len(uris) == 0 || len(uris) > maxLibraryURIs {
		return nil, fmt.Errorf("need 1 to %d URIs, got %d", maxLibraryURIs, len(uris))
	}
	out := make([]string, len(uris))
	for i, uri := range uris {
		id, err := librespot.SpotifyIdFromUri(uri)
		if err != nil || !slices.Contains(types, id.Type()) {
			return nil, fmt.Errorf("not a %s URI: %q", types[0], uri)
		}
		out[i] = id.Uri()
	}
	return out, nil
}

func canonicalTrackURIs(uris []string) ([]string, error) {
	return canonicalURIs(uris, librespot.SpotifyIdTypeTrack)
}

// checkPlaylistURI checks that uri names a playlist.
func checkPlaylistURI(uri string) error {
	id, err := librespot.SpotifyIdFromUri(uri)
	if err != nil || id.Type() != librespot.SpotifyIdTypePlaylist {
		return fmt.Errorf("not a playlist URI: %q", uri)
	}
	return nil
}

// PlaylistAddTracks adds tracks or episodes to the end of a playlist. A
// playlist that the user cannot change gives an error that wraps
// daemon.ErrForbidden.
func (e *Engine) PlaylistAddTracks(ctx context.Context, playlistURI string, uris []string) error {
	if err := checkPlaylistURI(playlistURI); err != nil {
		return err
	}
	if _, err := canonicalURIs(uris, librespot.SpotifyIdTypeTrack, librespot.SpotifyIdTypeEpisode); err != nil {
		return err
	}
	_, err := e.call(ctx, daemon.ApiRequestTypePlaylistAddTracks, daemon.ApiPlaylistAddTracks{PlaylistUri: playlistURI, Uris: uris})
	return err
}

// PlaylistRemoveTrack removes one item from a playlist. Without a position
// it removes the first copy of uri. With a position, the item at that
// index of the playlist must be uri. If it is not, the error wraps
// spclient.ErrPlaylistConflict. A playlist that the user cannot change
// gives an error that wraps daemon.ErrForbidden.
//
// go-librespot v0.10.5 checks the position against the full playlist, but
// ContextTracks leaves out local files. So a position from ContextTracks
// is only right when no local file comes before the item.
func (e *Engine) PlaylistRemoveTrack(ctx context.Context, playlistURI, uri string, position *int) error {
	if err := checkPlaylistURI(playlistURI); err != nil {
		return err
	}
	if _, err := canonicalURIs([]string{uri}, librespot.SpotifyIdTypeTrack, librespot.SpotifyIdTypeEpisode); err != nil {
		return err
	}
	// go-librespot indexes the playlist with the position without a check,
	// so a negative position panics and stops the process.
	if position != nil && *position < 0 {
		return fmt.Errorf("negative playlist position %d", *position)
	}
	_, err := e.call(ctx, daemon.ApiRequestTypePlaylistRemoveTrack,
		daemon.ApiPlaylistRemoveTrack{PlaylistUri: playlistURI, Uri: uri, Position: position})
	return err
}

// SetLiked adds the tracks to Liked Songs, or removes them when liked is
// false. go-librespot does not make the URIs canonical for this request, so
// the wrapper does.
func (e *Engine) SetLiked(ctx context.Context, uris []string, liked bool) error {
	canonical, err := canonicalTrackURIs(uris)
	if err != nil {
		return err
	}
	_, err = e.call(ctx, daemon.ApiRequestTypeSetLiked, daemon.ApiSetLiked{Uris: canonical, Liked: liked})
	return err
}

// LikedStates reports for each track whether it is in Liked Songs. The map
// keys are the URIs as given.
func (e *Engine) LikedStates(ctx context.Context, uris []string) (map[string]bool, error) {
	if _, err := canonicalTrackURIs(uris); err != nil {
		return nil, err
	}
	resp, err := e.call(ctx, daemon.ApiRequestTypeGetLiked, uris)
	if err != nil {
		return nil, err
	}
	states, ok := resp.(*daemon.ApiLikedStates)
	if !ok {
		return nil, fmt.Errorf("unexpected liked reply %T", resp)
	}
	out := make(map[string]bool, len(states.Items))
	for _, s := range states.Items {
		out[s.Uri] = s.Liked
	}
	return out, nil
}
