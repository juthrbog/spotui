package webapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestSetAlbumsSavedUsesLibrary(t *testing.T) {
	var methods []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library" || r.URL.Query().Get("uris") != "spotify:album:a,spotify:album:b" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		methods = append(methods, r.Method)
		w.WriteHeader(http.StatusOK)
	})
	uris := []string{"spotify:album:a", "spotify:album:b"}
	if err := c.SetAlbumsSaved(context.Background(), uris, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAlbumsSaved(context.Background(), uris, false); err != nil {
		t.Fatal(err)
	}
	if strings.Join(methods, " ") != "PUT DELETE" {
		t.Errorf("methods = %v", methods)
	}
}

func TestAlbumsSaved(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/library/contains" || r.Method != http.MethodGet {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		_, _ = w.Write([]byte(`[true, false]`))
	})
	got, err := c.AlbumsSaved(context.Background(), []string{"spotify:album:a", "spotify:album:b"})
	if err != nil {
		t.Fatal(err)
	}
	if !got["spotify:album:a"] || got["spotify:album:b"] {
		t.Errorf("states = %v", got)
	}
}

func TestAlbumsSavedRejectsShortReply(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[true]`)) })
	if _, err := c.AlbumsSaved(context.Background(), []string{"spotify:album:a", "spotify:album:b"}); err == nil {
		t.Error("no error for a reply with too few states")
	}
}

func TestAlbumCallsCheckURIs(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { t.Errorf("unexpected request %s", r.URL) })
	many := make([]string, maxLibraryURIs+1)
	for i := range many {
		many[i] = "spotify:album:a"
	}
	for _, uris := range [][]string{nil, many, {"spotify:track:a"}, {"spotify:album:"}, {"spotify:album:a,b"}} {
		if err := c.SetAlbumsSaved(context.Background(), uris, true); err == nil {
			t.Errorf("SetAlbumsSaved(%d URIs %q) gave no error", len(uris), uris[0:min(len(uris), 1)])
		}
	}
}

func TestInsufficientScopeIsMissingScope(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": {"status": 403, "message": "Insufficient client scope"}}`))
	})
	err := c.SetAlbumsSaved(context.Background(), []string{"spotify:album:a"}, true)
	var apiErr *APIError
	if !errors.Is(err, ErrMissingScope) || !errors.As(err, &apiErr) {
		t.Errorf("err = %v, want ErrMissingScope that wraps the APIError", err)
	}
}

func TestScopesAreSavedWithToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	c := New("id", 8898, path)
	tok := (&oauth2.Token{AccessToken: "a", RefreshToken: "r"}).WithExtra(map[string]any{"scope": "user-library-read user-library-modify"})
	if err := c.saveToken(tok); err != nil {
		t.Fatal(err)
	}

	loaded := New("id", 8898, path)
	if !loaded.HasScope(ScopeLibraryModify) || !loaded.HasScope("user-library-read") {
		t.Errorf("scopes after load = %q", loaded.scope)
	}

	// A refreshed token without scopes keeps the scopes of the login.
	if err := loaded.saveToken(&oauth2.Token{AccessToken: "b", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	if !New("id", 8898, path).HasScope(ScopeLibraryModify) {
		t.Error("a refresh lost the scopes")
	}
}

func TestOldTokenFileHasNoScopes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(path, []byte(`{"access_token": "a", "refresh_token": "r"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := New("id", 8898, path)
	if !c.LoggedIn() || c.HasScope(ScopeLibraryModify) {
		t.Errorf("logged in %v, modify scope %v; want logged in without the scope", c.LoggedIn(), c.HasScope(ScopeLibraryModify))
	}
	if err := c.Logout(); err != nil || c.HasScope("user-library-read") {
		t.Errorf("after logout: err %v, scope %q", err, c.scope)
	}
}
