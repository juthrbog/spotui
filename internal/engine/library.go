package engine

import (
	"context"
	"fmt"

	librespot "github.com/devgianlu/go-librespot"
	"github.com/devgianlu/go-librespot/daemon"
)

// The HTTP server of go-librespot checks the data of library requests, but
// requests over the channel skip those checks, and some bad data makes the
// player panic. The wrappers in this file make the same checks.

// maxLibraryURIs is the most URIs that go-librespot takes in one library
// request.
const maxLibraryURIs = 50

// canonicalTrackURIs checks that uris holds 1 to 50 track URIs and returns
// them in canonical form.
func canonicalTrackURIs(uris []string) ([]string, error) {
	if len(uris) == 0 || len(uris) > maxLibraryURIs {
		return nil, fmt.Errorf("need 1 to %d track URIs, got %d", maxLibraryURIs, len(uris))
	}
	out := make([]string, len(uris))
	for i, uri := range uris {
		id, err := librespot.SpotifyIdFromUri(uri)
		if err != nil || id.Type() != librespot.SpotifyIdTypeTrack {
			return nil, fmt.Errorf("not a track URI: %q", uri)
		}
		out[i] = id.Uri()
	}
	return out, nil
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
