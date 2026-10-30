package webapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// maxLibraryURIs is the most URIs that one /me/library call takes.
const maxLibraryURIs = 40

func checkAlbumURIs(uris []string) error {
	if len(uris) == 0 || len(uris) > maxLibraryURIs {
		return fmt.Errorf("need 1 to %d album URIs, got %d", maxLibraryURIs, len(uris))
	}
	for _, uri := range uris {
		id, ok := strings.CutPrefix(uri, "spotify:album:")
		if !ok || id == "" || strings.ContainsAny(id, ",/?#") {
			return fmt.Errorf("not an album URI: %q", uri)
		}
	}
	return nil
}

// SetAlbumsSaved saves the albums to the library of the user, or removes
// them when saved is false. It needs ScopeLibraryModify. The February 2026
// API removed PUT and DELETE /me/albums, so it uses /me/library.
func (c *Client) SetAlbumsSaved(ctx context.Context, uris []string, saved bool) error {
	if err := checkAlbumURIs(uris); err != nil {
		return err
	}
	method := http.MethodPut
	if !saved {
		method = http.MethodDelete
	}
	return c.do(ctx, method, "/me/library", url.Values{"uris": {strings.Join(uris, ",")}}, nil)
}

// AlbumsSaved reports for each album whether it is in the library of the
// user. The map keys are the URIs as given.
func (c *Client) AlbumsSaved(ctx context.Context, uris []string) (map[string]bool, error) {
	if err := checkAlbumURIs(uris); err != nil {
		return nil, err
	}
	var states []bool
	if err := c.get(ctx, "/me/library/contains", url.Values{"uris": {strings.Join(uris, ",")}}, &states); err != nil {
		return nil, err
	}
	if len(states) != len(uris) {
		return nil, fmt.Errorf("spotify web api gave %d states for %d albums", len(states), len(uris))
	}
	out := make(map[string]bool, len(uris))
	for i, uri := range uris {
		out[uri] = states[i]
	}
	return out, nil
}
