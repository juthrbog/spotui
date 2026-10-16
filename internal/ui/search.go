package ui

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/juthrbog/spotui/internal/webapi"
)

// searchPage searches Spotify through the Web API.
type searchPage struct {
	input        textinput.Model
	inputFocused bool
	list         list

	results *webapi.SearchResult
	err     error
	loading bool
	seq     int    // ignores replies to older searches
	pending string // query to run once the Web API login finishes
}

func newSearchPage() *searchPage {
	in := textinput.New()
	in.Prompt = "Search: "
	in.Placeholder = "songs, albums, artists, playlists"
	in.CharLimit = 200
	return &searchPage{
		input: in,
		list: list{columns: []column{
			{weight: 4},
			{weight: 3},
			{weight: 3},
			{width: 7, right: true},
		}},
	}
}

func (p *searchPage) title() string { return "Search" }
func (p *searchPage) typing() bool  { return p.inputFocused }

func (p *searchPage) selection() *row {
	if p.results == nil || p.loading {
		return nil
	}
	return p.list.selected()
}

func (p *searchPage) focusInput() tea.Cmd {
	p.inputFocused = true
	return p.input.Focus()
}

func (p *searchPage) blurInput() {
	p.inputFocused = false
	p.input.Blur()
}

// run starts a search, or a Web API login first if needed.
func (p *searchPage) run(m *Model, query string) tea.Cmd {
	if !m.web.Configured() {
		return nil
	}
	if !m.web.LoggedIn() {
		p.pending = query
		return m.startWebLogin()
	}
	p.pending = ""
	p.seq++
	p.loading = true
	p.err = nil
	return search(m.ctx, m.web, query, p.seq)
}

func (p *searchPage) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case searchMsg:
		if msg.seq != p.seq {
			return nil
		}
		p.loading = false
		if errors.Is(msg.err, webapi.ErrNotLoggedIn) {
			p.pending = strings.TrimSpace(p.input.Value())
			return m.startWebLogin()
		}
		p.results, p.err = msg.res, msg.err
		p.rebuild()
		return nil
	case tea.KeyPressMsg:
		if p.inputFocused {
			switch msg.String() {
			case "enter":
				query := strings.TrimSpace(p.input.Value())
				if query == "" {
					return nil
				}
				p.blurInput()
				return p.run(m, query)
			case "esc":
				p.blurInput()
				return nil
			case "down":
				if p.results != nil {
					p.blurInput()
				}
				return nil
			}
			var cmd tea.Cmd
			p.input, cmd = p.input.Update(msg)
			return cmd
		}
		if p.list.handleKey(msg.String()) {
			return nil
		}
		switch msg.String() {
		case "enter":
			r := p.list.selected()
			switch {
			case r == nil:
			case r.play:
				return m.play(r.uri, "")
			default:
				return m.push(newTracksPage(r.uri, r.label))
			}
		case "i", "esc":
			return p.focusInput()
		}
		return nil
	}

	// Cursor blink and paste messages.
	if p.inputFocused {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd
	}
	return nil
}

func (p *searchPage) rebuild() {
	res := p.results
	if res == nil {
		p.list.setRows(nil)
		return
	}
	var rows []row
	section := func(title string) {
		if len(rows) > 0 {
			rows = append(rows, row{kind: rowHeader})
		}
		rows = append(rows, row{kind: rowHeader, cols: []string{title}})
	}

	if len(res.Tracks.Items) > 0 {
		section("Songs")
		for _, t := range res.Tracks.Items {
			artists := webapi.ArtistNames(t.Artists)
			rows = append(rows, row{
				cols:  []string{t.Name, artists, t.Album.Name, formatMs(int64(t.DurationMs))},
				uri:   t.URI,
				play:  true,
				image: webapi.PickImage(t.Album.Images),
				info:  []string{t.Name, artists, t.Album.Name},
			})
		}
	}
	if len(res.Albums.Items) > 0 {
		section("Albums")
		for _, a := range res.Albums.Items {
			artists := webapi.ArtistNames(a.Artists)
			rows = append(rows, row{
				cols:  []string{a.Name, artists, a.Year()},
				uri:   a.URI,
				label: a.Name,
				image: webapi.PickImage(a.Images),
				info:  []string{a.Name, artists, albumKind(a)},
			})
		}
	}
	if len(res.Artists.Items) > 0 {
		section("Artists")
		for _, a := range res.Artists.Items {
			rows = append(rows, row{
				cols:  []string{a.Name, "top tracks"},
				uri:   a.URI,
				label: a.Name,
				image: webapi.PickImage(a.Images),
				info:  []string{a.Name, "Artist"},
			})
		}
	}
	var playlists []row
	for _, pl := range res.Playlists.Items {
		if pl == nil {
			continue
		}
		count := ""
		if n := pl.TrackCount(); n >= 0 {
			count = plural(n, "track")
		}
		owner := "by " + pl.Owner.DisplayName
		playlists = append(playlists, row{
			cols:  []string{pl.Name, owner, count},
			uri:   pl.URI,
			label: pl.Name,
			image: webapi.PickImage(pl.Images),
			info:  []string{pl.Name, "Playlist · " + owner, count},
		})
	}
	if len(playlists) > 0 {
		section("Playlists")
		rows = append(rows, playlists...)
	}
	if len(rows) == 0 {
		rows = append(rows, note("No results."))
	}
	p.list.cursor = 0
	p.list.offset = 0
	p.list.setRows(rows)
}

func (p *searchPage) view(m *Model, width, height int) string {
	p.input.SetWidth(max(width-len(p.input.Prompt)-1, 10))
	head := p.input.View() + "\n"
	bodyHeight := height - 2

	var body string
	switch {
	case !m.web.Configured():
		body = setupHelp(m, width)
	case p.pending != "":
		body = styleDim.Render("Waiting for the Web API login, then searching for \"" + p.pending + "\"…")
	case p.loading:
		body = styleDim.Render("Searching…")
	case p.err != nil:
		body = styleError.Render("Search failed: " + p.err.Error())
	case p.results == nil && p.inputFocused:
		body = styleDim.Render("Type a search and press enter. Press esc to stop typing.")
	case p.results == nil:
		body = styleDim.Render("Press / to type a search.")
	default:
		body = p.list.view(width, bodyHeight, !p.inputFocused, m.playingURI())
	}
	return head + "\n" + body
}

func setupHelp(m *Model, width int) string {
	lines := []string{
		styleBold.Render("Search needs your own Spotify developer app."),
		"It is free and takes about two minutes:",
		"",
		"  1. Open " + link("https://developer.spotify.com/dashboard", width-10) + " and create an app.",
		"  2. Add this redirect URI: " + styleAccent.Render(m.info.RedirectURL),
		"  3. Under \"Which API/SDKs are you planning to use?\", select Web API.",
		"  4. Copy the app's client ID into client_id in:",
		"     " + styleAccent.Render(m.info.ConfigFile),
		"  5. Restart spotui.",
		"",
		styleDim.Render("Playback and your playlists work without it."),
	}
	return strings.Join(lines, "\n")
}
