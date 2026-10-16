package webapi

import "strings"

// Image is one size of a cover or artist picture.
type Image struct {
	URL   string `json:"url"`
	Width *int   `json:"width"`
}

// coverWidth is the image width that fits the art panel best.
const coverWidth = 300

// PickImage returns the URL of the smallest image that is at least 300
// pixels wide, or of the first image if no width is known.
func PickImage(images []Image) string {
	bestURL, bestWidth := "", 0
	for _, img := range images {
		if img.Width == nil || *img.Width < coverWidth {
			continue
		}
		if bestURL == "" || *img.Width < bestWidth {
			bestURL, bestWidth = img.URL, *img.Width
		}
	}
	if bestURL == "" && len(images) > 0 {
		return images[0].URL
	}
	return bestURL
}

// Artist is a Spotify artist. Artists inside track and album objects have
// no images.
type Artist struct {
	URI    string  `json:"uri"`
	Name   string  `json:"name"`
	Images []Image `json:"images"`
}

// Album is a Spotify album.
type Album struct {
	URI         string   `json:"uri"`
	Name        string   `json:"name"`
	Artists     []Artist `json:"artists"`
	ReleaseDate string   `json:"release_date"`
	TotalTracks int      `json:"total_tracks"`
	Images      []Image  `json:"images"`
}

// Year returns the release year, or an empty string.
func (a Album) Year() string {
	if len(a.ReleaseDate) >= 4 {
		return a.ReleaseDate[:4]
	}
	return ""
}

// Track is a Spotify track.
type Track struct {
	URI        string   `json:"uri"`
	Name       string   `json:"name"`
	DurationMs int      `json:"duration_ms"`
	Artists    []Artist `json:"artists"`
	Album      Album    `json:"album"`
}

// Playlist is a Spotify playlist.
type Playlist struct {
	URI    string  `json:"uri"`
	Name   string  `json:"name"`
	Images []Image `json:"images"`
	Owner  struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	// The February 2026 API renamed "tracks" to "items". Both are read.
	Tracks *struct {
		Total int `json:"total"`
	} `json:"tracks"`
	Items *struct {
		Total int `json:"total"`
	} `json:"items"`
}

// TrackCount returns the number of tracks, or -1 if unknown.
func (p Playlist) TrackCount() int {
	switch {
	case p.Items != nil:
		return p.Items.Total
	case p.Tracks != nil:
		return p.Tracks.Total
	default:
		return -1
	}
}

// Page is one page of results.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// SearchResult holds the results of a search. Spotify sometimes puts null
// entries in the playlist results, so they are pointers.
type SearchResult struct {
	Tracks    Page[Track]     `json:"tracks"`
	Albums    Page[Album]     `json:"albums"`
	Artists   Page[Artist]    `json:"artists"`
	Playlists Page[*Playlist] `json:"playlists"`
}

// ArtistNames joins artist names with commas.
func ArtistNames(artists []Artist) string {
	names := make([]string, len(artists))
	for i, a := range artists {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}
