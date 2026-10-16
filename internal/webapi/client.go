package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"golang.org/x/oauth2"
)

const apiBase = "https://api.spotify.com/v1"

// RateLimitError means Spotify rejected the call because of its rate limit.
type RateLimitError struct {
	RetryAfter time.Duration
	Reason     string
}

func (e *RateLimitError) Error() string {
	msg := "spotify rate limit reached"
	if e.Reason == "QUOTA_EXCEEDED" {
		msg = "spotify quota for this developer app is used up"
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s, try again in %s", msg, e.RetryAfter.Round(time.Second))
	}
	return msg
}

// APIError is an error reply from the Web API.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("spotify web api: %s (HTTP %d)", e.Message, e.Status)
}

// maxRetryWait is the longest Retry-After the client waits out by itself.
const maxRetryWait = 5 * time.Second

// get calls GET path and decodes the JSON reply into out.
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	c.mu.Lock()
	ts := c.ts
	c.mu.Unlock()
	if ts == nil {
		return ErrNotLoggedIn
	}

	tok, err := ts.Token()
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) && re.Response != nil && re.Response.StatusCode < 500 {
			// The refresh token was revoked or expired. A new login fixes it.
			_ = c.Logout()
			return ErrNotLoggedIn
		}
		return fmt.Errorf("refreshing Web API token: %w", err)
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("calling spotify web api: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("reading spotify web api reply: %w", err)
		}

		switch resp.StatusCode {
		case http.StatusOK:
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("parsing spotify web api reply: %w", err)
			}
			return nil
		case http.StatusTooManyRequests:
			rle := &RateLimitError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
			var e struct {
				Error struct {
					Reason string `json:"reason"`
				} `json:"error"`
			}
			_ = json.Unmarshal(body, &e)
			rle.Reason = e.Error.Reason
			if attempt == 0 && rle.Reason != "QUOTA_EXCEEDED" && rle.RetryAfter <= maxRetryWait {
				select {
				case <-time.After(max(rle.RetryAfter, 500*time.Millisecond)):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return rle
		case http.StatusUnauthorized:
			_ = c.Logout()
			return ErrNotLoggedIn
		default:
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			_ = json.Unmarshal(body, &e)
			if e.Error.Message == "" {
				e.Error.Message = http.StatusText(resp.StatusCode)
			}
			return &APIError{Status: resp.StatusCode, Message: e.Error.Message}
		}
	}
}

func parseRetryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// Search finds tracks, albums, artists, and playlists. Development-mode apps
// get at most 10 results per type.
func (c *Client) Search(ctx context.Context, query string) (*SearchResult, error) {
	q := url.Values{
		"q":     {query},
		"type":  {"track,album,artist,playlist"},
		"limit": {"10"},
	}
	var res SearchResult
	if err := c.get(ctx, "/search", q, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// maxSavedAlbums caps how many saved albums are loaded, to keep the number
// of calls low.
const maxSavedAlbums = 500

// SavedAlbums returns the albums in the user's library, newest first.
func (c *Client) SavedAlbums(ctx context.Context) ([]Album, error) {
	var albums []Album
	for offset := 0; offset < maxSavedAlbums; {
		var page struct {
			Items []struct {
				Album Album `json:"album"`
			} `json:"items"`
			Next  string `json:"next"`
			Total int    `json:"total"`
		}
		q := url.Values{"limit": {"50"}, "offset": {strconv.Itoa(offset)}}
		if err := c.get(ctx, "/me/albums", q, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			albums = append(albums, it.Album)
		}
		if page.Next == "" || len(page.Items) == 0 {
			break
		}
		offset += len(page.Items)
	}
	return albums, nil
}
