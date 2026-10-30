package webapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/oauth2"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("test-client", 8898, filepath.Join(t.TempDir(), "token.json"))
	c.baseURL = srv.URL
	c.ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
	return c
}

func TestSearchParsesResultsAndSkipsNullPlaylists(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("limit = %q, want 10", got)
		}
		_, _ = w.Write([]byte(`{
			"tracks": {"items": [{"uri": "spotify:track:1", "name": "Song", "duration_ms": 61000,
				"artists": [{"name": "A"}, {"name": "B"}], "album": {"name": "Record"}}]},
			"albums": {"items": [{"uri": "spotify:album:1", "name": "Record", "release_date": "1999-01-01",
				"images": [{"url": "https://i/640", "width": 640}, {"url": "https://i/300", "width": 300}, {"url": "https://i/64", "width": 64}]}]},
			"artists": {"items": []},
			"playlists": {"items": [null, {"uri": "spotify:playlist:1", "name": "Mix", "items": {"total": 7}}]}
		}`))
	})

	res, err := c.Search(context.Background(), "song")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tracks.Items) != 1 || ArtistNames(res.Tracks.Items[0].Artists) != "A, B" {
		t.Errorf("tracks = %+v", res.Tracks.Items)
	}
	if got := PickImage(res.Albums.Items[0].Images); got != "https://i/300" {
		t.Errorf("PickImage = %q, want the 300 px image", got)
	}
	if res.Albums.Items[0].Year() != "1999" {
		t.Errorf("year = %q", res.Albums.Items[0].Year())
	}
	if len(res.Playlists.Items) != 2 || res.Playlists.Items[0] != nil {
		t.Fatalf("playlists = %+v", res.Playlists.Items)
	}
	if n := res.Playlists.Items[1].TrackCount(); n != 7 {
		t.Errorf("track count = %d, want 7", n)
	}
}

func TestRateLimitRetriesOnce(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})

	if _, err := c.Search(context.Background(), "x"); err != nil {
		t.Fatalf("Search after one 429: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestQuotaExceededIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": {"status": 429, "reason": "QUOTA_EXCEEDED"}}`))
	})

	_, err := c.Search(context.Background(), "x")
	var rle *RateLimitError
	if !errors.As(err, &rle) || rle.Reason != "QUOTA_EXCEEDED" {
		t.Fatalf("err = %v, want quota RateLimitError", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestUnauthorizedLogsOut(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	if err := os.WriteFile(c.tokenPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Search(context.Background(), "x"); !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("err = %v, want ErrNotLoggedIn", err)
	}
	if c.LoggedIn() {
		t.Error("client still logged in")
	}
	if _, err := os.Stat(c.tokenPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("token file still exists: %v", err)
	}
}

func TestAPIErrorMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": {"status": 403, "message": "Forbidden for this app"}}`))
	})

	_, err := c.SavedAlbums(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 403 || apiErr.Message != "Forbidden for this app" {
		t.Fatalf("err = %v", err)
	}
}

func TestSavedAlbumsFollowsPages(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") == "0" {
			_, _ = w.Write([]byte(`{"items": [{"album": {"name": "One"}}, {"album": {"name": "Two"}}], "next": "more"}`))
			return
		}
		_, _ = w.Write([]byte(`{"items": [{"album": {"name": "Three"}}], "next": null}`))
	})

	albums, err := c.SavedAlbums(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 3 || albums[2].Name != "Three" {
		t.Errorf("albums = %+v", albums)
	}
}

func TestArtistAlbumsFollowsPagesForOneGroup(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/artists/abc/albums" || q.Get("include_groups") != GroupSingle || q.Get("limit") != "10" {
			t.Errorf("request = %s", r.URL)
		}
		if q.Get("offset") == "0" {
			_, _ = w.Write([]byte(`{"items": [{"name": "One"}, {"name": "Two"}], "total": 3}`))
			return
		}
		_, _ = w.Write([]byte(`{"items": [{"name": "Three"}], "total": 3}`))
	})

	albums, total, err := c.ArtistAlbums(context.Background(), "spotify:artist:abc", GroupSingle)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 3 || albums[2].Name != "Three" || total != 3 {
		t.Errorf("albums = %+v, total = %d", albums, total)
	}
}

func TestArtistAlbumsStopsAtCap(t *testing.T) {
	// Short pages of 9 albums must not take the result over the cap.
	var calls atomic.Int32
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if n > 10 {
			t.Errorf("limit = %d, the API allows 10 at most", n)
		}
		items := strings.TrimSuffix(strings.Repeat(`{"name": "A"},`, min(n, 9)), ",")
		_, _ = w.Write([]byte(`{"items": [` + items + `], "total": 1000}`))
	})

	albums, total, err := c.ArtistAlbums(context.Background(), "spotify:artist:abc", GroupAppearsOn)
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != maxArtistAlbums || total != 1000 || calls.Load() != 6 {
		t.Errorf("got %d albums, total %d, %d calls", len(albums), total, calls.Load())
	}
}

func TestArtistAlbumsRejectsOtherURIs(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL)
	})
	for _, uri := range []string{"spotify:album:abc", "spotify:artist:", "spotify:artist:a/b"} {
		if _, _, err := c.ArtistAlbums(context.Background(), uri, GroupAlbum); err == nil {
			t.Errorf("ArtistAlbums(%q) gave no error", uri)
		}
	}
}

func TestPickImageWithoutWidths(t *testing.T) {
	if got := PickImage([]Image{{URL: "https://mosaic"}}); got != "https://mosaic" {
		t.Errorf("PickImage = %q", got)
	}
	if got := PickImage(nil); got != "" {
		t.Errorf("PickImage(nil) = %q", got)
	}
}

func TestNotConfigured(t *testing.T) {
	c := New("", 8898, filepath.Join(t.TempDir(), "token.json"))
	if _, err := c.Search(context.Background(), "x"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
	if _, err := c.BeginLogin(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("BeginLogin err = %v, want ErrNotConfigured", err)
	}
}

func TestSavedTokenIsLoaded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	c := New("id", 8898, path)
	if c.LoggedIn() {
		t.Fatal("logged in without a token file")
	}
	if err := c.saveToken(&oauth2.Token{AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %o, want 600", perm)
	}
	if !New("id", 8898, path).LoggedIn() {
		t.Error("saved token was not loaded")
	}
}
