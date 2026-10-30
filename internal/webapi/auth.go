// Package webapi is a small Spotify Web API client for search and saved
// albums. It logs in with the user's own developer app through the
// Authorization Code flow with PKCE, so it needs no client secret.
package webapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/juthrbog/spotui/internal/atomicfile"
)

var (
	// ErrNotConfigured means no client ID is set.
	ErrNotConfigured = errors.New("no Spotify developer app client ID is set")
	// ErrNotLoggedIn means there is no valid Web API token.
	ErrNotLoggedIn = errors.New("not logged in to the Spotify Web API")
	// ErrMissingScope means the login does not allow the call. The login is
	// older than a scope that the call needs, so a new login fixes it.
	ErrMissingScope = errors.New("the Web API login does not allow this")
)

// ScopeLibraryModify allows changes to the library of the user, such as
// saving albums.
const ScopeLibraryModify = "user-library-modify"

// scopes are the scopes that a login asks for. A login asks for all of
// them, so a new feature needs one new login at most.
var scopes = []string{"user-library-read", ScopeLibraryModify}

var endpoint = oauth2.Endpoint{ // #nosec G101 -- public OAuth URLs, no credentials
	AuthURL:  "https://accounts.spotify.com/authorize",
	TokenURL: "https://accounts.spotify.com/api/token",
	// PKCE clients have no secret, so the client ID goes in the form body.
	AuthStyle: oauth2.AuthStyleInParams,
}

// Client calls the Spotify Web API.
type Client struct {
	oauth     *oauth2.Config
	tokenPath string
	http      *http.Client
	port      int
	baseURL   string

	mu    sync.Mutex
	ts    oauth2.TokenSource // nil when not logged in
	scope string             // scopes that the login allows, separated by spaces
}

// New creates a client. It loads a saved token from tokenPath if one exists.
func New(clientID string, port int, tokenPath string) *Client {
	c := &Client{
		tokenPath: tokenPath,
		port:      port,
		baseURL:   apiBase,
		http:      &http.Client{Timeout: 20 * time.Second},
		oauth: &oauth2.Config{
			ClientID:    clientID,
			Endpoint:    endpoint,
			RedirectURL: "http://127.0.0.1:" + strconv.Itoa(port) + "/callback",
			Scopes:      scopes,
		},
	}
	if saved, err := c.loadToken(); err == nil {
		c.ts = c.tokenSource(saved.Token)
		c.scope = saved.Scope
	}
	return c
}

// HasScope reports whether the login allows scope. A token that spotui
// saved before it kept the scopes counts as allowing none.
func (c *Client) HasScope(scope string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ts != nil && slices.Contains(strings.Fields(c.scope), scope)
}

// Configured reports whether a client ID is set.
func (c *Client) Configured() bool { return c.oauth.ClientID != "" }

// LoggedIn reports whether the client has a token.
func (c *Client) LoggedIn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ts != nil
}

// Login is a login in progress. Show URL to the user, then call Wait.
type Login struct {
	URL string

	client   *Client
	verifier string
	state    string
	server   *http.Server
	result   chan loginResult
}

type loginResult struct {
	code string
	err  error
}

// BeginLogin starts the callback server and returns the URL the user must
// open in a browser.
func (c *Client) BeginLogin() (*Login, error) {
	if !c.Configured() {
		return nil, ErrNotConfigured
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("creating login state: %w", err)
	}
	l := &Login{
		client:   c,
		verifier: oauth2.GenerateVerifier(),
		state:    hex.EncodeToString(stateBytes),
		result:   make(chan loginResult, 1),
	}
	l.URL = c.oauth.AuthCodeURL(l.state, oauth2.S256ChallengeOption(l.verifier))

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.port)))
	if err != nil {
		return nil, fmt.Errorf("listening for the login callback on port %d: %w", c.port, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", l.handleCallback)
	l.server = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = l.server.Serve(ln) }()
	return l, nil
}

func (l *Login) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var res loginResult
	switch {
	case q.Get("state") != l.state:
		http.Error(w, "Login state does not match. Start the login again from spotui.", http.StatusBadRequest)
		return
	case q.Get("error") != "":
		res.err = fmt.Errorf("spotify refused the login: %s", q.Get("error"))
	case q.Get("code") == "":
		http.Error(w, "The login reply has no code.", http.StatusBadRequest)
		return
	default:
		res.code = q.Get("code")
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if res.err != nil {
		_, _ = fmt.Fprint(w, "<p>Login failed. You can close this tab and look at spotui for details.</p>")
	} else {
		_, _ = fmt.Fprint(w, "<p>spotui is logged in. You can close this tab.</p>")
	}
	select {
	case l.result <- res:
	default:
	}
}

// Wait blocks until the browser login finishes, then saves the token.
func (l *Login) Wait(ctx context.Context) error {
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = l.server.Shutdown(shutdownCtx)
	}()

	var res loginResult
	select {
	case res = <-l.result:
	case <-ctx.Done():
		return ctx.Err()
	}
	if res.err != nil {
		return res.err
	}

	c := l.client
	tok, err := c.oauth.Exchange(c.oauthContext(ctx), res.code, oauth2.VerifierOption(l.verifier))
	if err != nil {
		return fmt.Errorf("getting the Web API token: %w", err)
	}
	if err := c.saveToken(tok); err != nil {
		return err
	}
	c.mu.Lock()
	c.ts = c.tokenSource(tok)
	c.mu.Unlock()
	return nil
}

// Logout forgets the token and deletes the token file.
func (c *Client) Logout() error {
	c.mu.Lock()
	c.ts, c.scope = nil, ""
	c.mu.Unlock()
	if err := os.Remove(c.tokenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing Web API token: %w", err)
	}
	return nil
}

func (c *Client) oauthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.http)
}

// tokenSource refreshes the token when it expires and saves each new token.
// Spotify does not always return a new refresh token; oauth2 then keeps the
// old one.
func (c *Client) tokenSource(tok *oauth2.Token) oauth2.TokenSource {
	base := c.oauth.TokenSource(c.oauthContext(context.Background()), tok)
	return &savingTokenSource{base: base, last: tok.AccessToken, save: c.saveToken}
}

type savingTokenSource struct {
	base oauth2.TokenSource
	save func(*oauth2.Token) error

	mu   sync.Mutex
	last string
}

func (s *savingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tok.AccessToken != s.last {
		s.last = tok.AccessToken
		_ = s.save(tok) // A failed save only costs a login on the next start.
	}
	return tok, nil
}

// savedToken is the token file: the token and the scopes that it allows.
type savedToken struct {
	*oauth2.Token
	Scope string `json:"scope,omitempty"`
}

func (c *Client) loadToken() (*savedToken, error) {
	b, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return nil, err
	}
	saved := savedToken{Token: &oauth2.Token{}}
	if err := json.Unmarshal(b, &saved); err != nil {
		return nil, err
	}
	if saved.RefreshToken == "" {
		return nil, errors.New("saved token has no refresh token")
	}
	return &saved, nil
}

// saveToken saves tok with its scopes. A refreshed token often does not
// name its scopes, and then the scopes of the login stay.
func (c *Client) saveToken(tok *oauth2.Token) error {
	scope, _ := tok.Extra("scope").(string)
	c.mu.Lock()
	if scope != "" {
		c.scope = scope
	} else {
		scope = c.scope
	}
	c.mu.Unlock()
	b, err := json.Marshal(savedToken{Token: tok, Scope: scope}) // #nosec G117 -- the token is meant to be saved; atomicfile makes the file 0600
	if err != nil {
		return err
	}
	if err := atomicfile.Write(c.tokenPath, b); err != nil {
		return fmt.Errorf("saving Web API token: %w", err)
	}
	return nil
}
