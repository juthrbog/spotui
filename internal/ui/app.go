// Package ui is the Bubble Tea terminal interface of spotui.
package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/devgianlu/go-librespot/daemon"

	"github.com/juthrbog/spotui/internal/engine"
	"github.com/juthrbog/spotui/internal/webapi"
)

// Info holds settings, paths, and URLs the interface needs.
type Info struct {
	ConfigFile  string
	LogFile     string
	RedirectURL string
	CoverArt    string // "auto", "kitty", "blocks", or "off"
}

type tab int

const (
	tabLibrary tab = iota
	tabSearch
)

// page is one screen in a tab. Each tab keeps a stack of pages; esc goes back.
type page interface {
	title() string
	update(m *Model, msg tea.Msg) tea.Cmd
	view(m *Model, width, height int) string
	typing() bool    // true while a text input has focus
	stopTyping()     // moves focus from the text input to the list
	selection() *row // the selected row, or nil
}

// Model is the root Bubble Tea model.
type Model struct {
	ctx  context.Context
	eng  *engine.Engine
	web  *webapi.Client
	info Info

	width, height int
	tab           tab
	stacks        [2][]page
	library       *libraryPage
	search        *searchPage

	status *daemon.ApiStatus
	ready  bool // the player is logged in and answering
	auth   *engine.AuthCode
	art    *artState

	webLoginBusy   bool
	webLoginURL    string
	webLoginCancel context.CancelFunc
	webLoginTried  bool

	flash    string
	flashErr bool
	flashID  int
	help     bool
	menu     *actionMenu
	confirm  *confirmPrompt

	liked       map[string]bool // track URI -> in Liked Songs
	likedAsked  map[string]bool // track URIs asked for or loaded
	likePending map[string]bool // track URIs with a like change on the way

	albums       albumStates     // album URI -> in the library of the user
	albumPending map[string]bool // album URIs with a change on the way
}

// New creates the root model.
func New(ctx context.Context, eng *engine.Engine, web *webapi.Client, info Info) *Model {
	m := &Model{
		ctx: ctx, eng: eng, web: web, info: info, art: newArtState(info.CoverArt),
		liked: map[string]bool{}, likedAsked: map[string]bool{}, likePending: map[string]bool{},
		albums: albumStates{saved: map[string]bool{}}, albumPending: map[string]bool{},
	}
	m.library = newLibraryPage()
	m.search = newSearchPage()
	m.stacks[tabLibrary] = []page{m.library}
	m.stacks[tabSearch] = []page{m.search}
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(waitForEvent(m.eng), fetchStatus(m.ctx, m.eng, true), m.art.init())
}

func (m *Model) current() page {
	s := m.stacks[m.tab]
	return s[len(s)-1]
}

func (m *Model) username() string {
	if m.status == nil {
		return ""
	}
	return m.status.Username
}

func (m *Model) playingURI() string {
	if m.status == nil || m.status.Track == nil {
		return ""
	}
	return m.status.Track.Uri
}

func (m *Model) push(p page) tea.Cmd {
	m.stacks[m.tab] = append(m.stacks[m.tab], p)
	if in, ok := p.(interface{ init(*Model) tea.Cmd }); ok {
		return in.init(m)
	}
	return nil
}

func (m *Model) pop() bool {
	s := m.stacks[m.tab]
	if len(s) <= 1 {
		return false
	}
	m.stacks[m.tab] = s[:len(s)-1]
	return true
}

func (m *Model) onStack(p page) bool {
	for _, s := range m.stacks {
		for _, q := range s {
			if q == p {
				return true
			}
		}
	}
	return false
}

func (m *Model) setFlash(text string, isErr bool) tea.Cmd {
	m.flashID++
	m.flash, m.flashErr = text, isErr
	id := m.flashID
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return flashClearMsg{id: id} })
}

// control runs a player command and refreshes the status after it.
func (m *Model) control(f func(context.Context) error) tea.Cmd {
	if !m.ready {
		return m.setFlash("The player is not ready yet.", true)
	}
	parent := m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()
		return actionDoneMsg{err: f(ctx)}
	}
}

func (m *Model) play(uri, skipTo string) tea.Cmd {
	return m.control(func(ctx context.Context) error { return m.eng.Play(ctx, uri, skipTo) })
}

func (m *Model) startWebLogin() tea.Cmd {
	if !m.web.Configured() {
		return m.setFlash("Set client_id in "+m.info.ConfigFile+" first.", true)
	}
	if m.webLoginBusy {
		return nil
	}
	m.webLoginBusy = true
	m.webLoginTried = true
	return func() tea.Msg {
		l, err := m.web.BeginLogin()
		return webLoginStartedMsg{login: l, err: err}
	}
}

// Update handles msg, then fetches or sends the cover art the next frame
// needs.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	return m, tea.Batch(cmd, m.art.sync(m.ctx, m.artWants()))
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	if ok, cmd := m.art.update(msg); ok {
		return cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m.art.resize()

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case engineEventMsg:
		return tea.Batch(m.handleEngineEvent(msg.ev), waitForEvent(m.eng))

	case statusTickMsg:
		return fetchStatus(m.ctx, m.eng, true)

	case statusMsg:
		return m.handleStatus(msg)

	case actionDoneMsg:
		var cmd tea.Cmd
		if msg.err != nil {
			cmd = m.setFlash(msg.err.Error(), true)
		}
		return tea.Batch(cmd, fetchStatus(m.ctx, m.eng, false))

	case albumSavedMsg:
		return m.albumSavedDone(msg)

	case queueAddMsg:
		return m.queueAdded(msg)

	case flashClearMsg:
		if msg.id != m.flashID {
			return nil
		}
		// The question hides the message, so keep it until the question
		// closes.
		if m.confirm != nil {
			return m.setFlash(m.flash, m.flashErr)
		}
		m.flash = ""
		return nil

	case webLoginStartedMsg:
		if msg.err != nil {
			m.webLoginBusy = false
			return m.setFlash("Web API login failed: "+msg.err.Error(), true)
		}
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
		m.webLoginURL, m.webLoginCancel = msg.login.URL, cancel
		openBrowser(msg.login.URL)
		login := msg.login
		return func() tea.Msg {
			defer cancel()
			return webLoginDoneMsg{err: login.Wait(ctx)}
		}

	case webLoginDoneMsg:
		m.webLoginBusy, m.webLoginURL, m.webLoginCancel = false, "", nil
		if msg.err != nil {
			if errors.Is(msg.err, context.Canceled) || errors.Is(msg.err, context.DeadlineExceeded) {
				return m.setFlash("Web API login cancelled. Press L to try again.", true)
			}
			return m.setFlash("Web API login failed: "+msg.err.Error(), true)
		}
		cmds := []tea.Cmd{m.setFlash("Logged in. Search and albums are ready.", false), m.library.loadAlbums(m), m.reloadArtistPages()}
		if q := m.search.pending; q != "" {
			cmds = append(cmds, m.search.run(m, q))
		}
		return tea.Batch(cmds...)
	}

	// Route other messages (async results, cursor blink) to every open page.
	// A page that was closed no longer gets its replies, which stops polling.
	if ct, ok := msg.(contextTracksMsg); ok && !m.onStack(ct.page) {
		return nil
	}
	cmds := []tea.Cmd{m.updateLiked(msg), m.updatePlaylists(msg)}
	for _, s := range m.stacks {
		for _, p := range s {
			cmds = append(cmds, p.update(m, msg))
		}
	}
	return tea.Batch(cmds...)
}

// quit frees the images in the terminal, then stops the program.
func (m *Model) quit() tea.Cmd {
	if seq := m.art.cleanup(); seq != "" {
		return tea.Sequence(tea.Raw(seq), tea.Quit)
	}
	return tea.Quit
}

func (m *Model) handleEngineEvent(ev any) tea.Cmd {
	switch ev := ev.(type) {
	case engine.AuthCode:
		if ev.URL == "" {
			m.auth = nil
			return nil
		}
		if m.auth == nil || m.auth.Code != ev.Code {
			openBrowser(ev.URL)
		}
		m.auth = &ev
		return nil
	case engine.Stopped:
		m.ready = false
		m.status = nil
		return m.setFlash(fmt.Sprintf("Player stopped: %v. Restarting in %s.", ev.Err, ev.Retry), true)
	case *daemon.ApiEvent:
		if ev.Type == daemon.ApiEventTypePlaybackError {
			if data, ok := ev.Data.(daemon.ApiEventDataPlaybackError); ok {
				return tea.Batch(m.setFlash("Playback error: "+data.Message, true), fetchStatus(m.ctx, m.eng, false))
			}
		}
		return fetchStatus(m.ctx, m.eng, false)
	}
	return nil
}

func (m *Model) handleStatus(msg statusMsg) tea.Cmd {
	var cmds []tea.Cmd
	if msg.poll {
		cmds = append(cmds, statusTick())
	}
	if msg.err != nil {
		return tea.Batch(cmds...)
	}
	// The player only answers once it is logged in.
	m.status = msg.st
	m.auth = nil
	if uri := m.playingURI(); uri != "" {
		cmds = append(cmds, m.wantLiked([]string{uri}))
	}
	if !m.ready {
		m.ready = true
		cmds = append(cmds, m.library.load(m))
		if m.web.Configured() && !m.web.LoggedIn() && !m.webLoginTried {
			cmds = append(cmds, m.startWebLogin())
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+c" {
		return m.quit()
	}
	if m.help {
		m.help = false
		return nil
	}
	if m.confirm != nil {
		return m.confirmKey(key)
	}
	if m.menu != nil {
		return m.menuKey(key)
	}
	if m.webLoginURL != "" && key == "esc" {
		if m.webLoginCancel != nil {
			m.webLoginCancel()
		}
		return nil
	}

	// Tab switches tabs even while typing, so a text box never traps the
	// keyboard.
	if key == "tab" || key == "shift+tab" {
		m.current().stopTyping()
		m.tab = 1 - m.tab
		return nil
	}

	cur := m.current()
	if cur.typing() {
		return cur.update(m, msg)
	}

	switch key {
	case "q":
		return m.quit()
	case "?":
		m.help = true
	case "space":
		return m.control(m.eng.PlayPause)
	case "n":
		return m.control(m.eng.Next)
	case "p":
		return m.control(m.eng.Prev)
	case "right":
		return m.control(func(ctx context.Context) error { return m.eng.Seek(ctx, 5000) })
	case "left":
		return m.control(func(ctx context.Context) error { return m.eng.Seek(ctx, -5000) })
	case "+", "=":
		return m.control(func(ctx context.Context) error { return m.eng.AdjustVolume(ctx, 5) })
	case "-", "_":
		return m.control(func(ctx context.Context) error { return m.eng.AdjustVolume(ctx, -5) })
	case "s":
		on := m.status == nil || !m.status.ShuffleContext
		return m.control(func(ctx context.Context) error { return m.eng.SetShuffle(ctx, on) })
	case "r":
		next := engine.RepeatContext
		if m.status != nil {
			switch {
			case m.status.RepeatTrack:
				next = engine.RepeatOff
			case m.status.RepeatContext:
				next = engine.RepeatTrack
			}
		}
		return m.control(func(ctx context.Context) error { return m.eng.SetRepeat(ctx, next) })
	case "1":
		m.tab = tabLibrary
	case "2":
		m.tab = tabSearch
	case "/":
		m.tab = tabSearch
		m.stacks[tabSearch] = m.stacks[tabSearch][:1]
		return m.search.focusInput()
	case "a":
		return m.openMenu()
	case "e":
		return m.addToQueue()
	case "l":
		return m.toggleLike()
	case "A":
		return m.goToAlbum()
	case "I":
		return m.goToArtist()
	case "o":
		return m.goToContext()
	case "L":
		return m.startWebLogin()
	case "R":
		if m.ready {
			return m.library.load(m)
		}
	case "esc", "backspace":
		// Esc removes a filter before it goes back.
		if f, ok := cur.(interface{ clearFilter() bool }); ok && key == "esc" && f.clearFilter() {
			return nil
		}
		if !m.pop() {
			return cur.update(m, msg)
		}
	default:
		return cur.update(m, msg)
	}
	return nil
}

// Layout: header, rule, breadcrumb, body, rule, three now-playing lines, footer.
const (
	chromeHeight = 8
	bodyTop      = 3 // first row of the body
)

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "spotui"
	if st := m.status; st != nil && st.Track != nil {
		v.WindowTitle = st.Track.Name + " · " + strings.Join(st.Track.ArtistNames, ", ")
	}
	return v
}

func (m *Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	w := m.width
	rule := styleDim.Render(strings.Repeat("─", w))
	bodyHeight := max(m.height-chromeHeight, 1)

	var body string
	switch {
	case m.help:
		body = helpView()
	case m.auth != nil:
		body = m.authView(w)
	case m.webLoginURL != "":
		body = m.webLoginView(w)
	case !m.ready:
		body = styleDim.Render("Connecting to Spotify…")
	case m.showPanel():
		panelWidth, _, _, _ := m.panelLayout()
		const sep = " │ "
		listWidth := w - panelWidth - lipgloss.Width(sep)
		body = joinColumns(
			m.overlayMenu(m.current().view(m, listWidth, bodyHeight), listWidth, bodyHeight),
			listWidth,
			styleDim.Render(sep),
			fitBlock(m.panelView(panelWidth), panelWidth, bodyHeight),
		)
	default:
		body = m.overlayMenu(m.current().view(m, w, bodyHeight), w, bodyHeight)
	}

	parts := []string{
		m.headerView(w),
		rule,
		m.crumbsView(w),
		fitBlock(body, w, bodyHeight),
		rule,
		fitBlock(m.nowPlayingView(w), w, 3),
		m.footerView(w),
	}
	return fitBlock(strings.Join(parts, "\n"), w, m.height)
}

func (m *Model) headerView(w int) string {
	tabs := []string{"1 Library", "2 Search"}
	for i, t := range tabs {
		if tab(i) == m.tab {
			tabs[i] = styleTabActive.Render(t)
		} else {
			tabs[i] = styleDim.Render(t)
		}
	}
	left := styleAccentBold.Render(" spotui") + "   " + strings.Join(tabs, "   ")

	var right string
	switch {
	case m.ready:
		right = styleAccent.Render("●") + " " + m.username() + " "
	case m.auth != nil:
		right = styleDim.Render("○ waiting for login ")
	default:
		right = styleDim.Render("○ connecting ")
	}
	return spread(left, right, w)
}

func (m *Model) crumbsView(w int) string {
	s := m.stacks[m.tab]
	titles := make([]string, len(s))
	for i, p := range s {
		titles[i] = p.title()
	}
	crumbs := " " + strings.Join(titles, " › ")
	if len(s) > 1 {
		crumbs += styleDim.Render("   (esc to go back)")
	}
	return styleBold.Render(fit(crumbs, w, false))
}

func (m *Model) nowPlayingView(w int) string {
	url := m.thumbURL()
	thumb, ok := m.art.view(artKey{url: url, cols: thumbCols, rows: thumbRows})
	if url == "" || !ok {
		return m.nowPlayingLines(w)
	}
	return joinColumns(thumb, thumbCols+1, "", m.nowPlayingLines(w-thumbCols-1))
}

func (m *Model) nowPlayingLines(w int) string {
	st := m.status
	if st == nil || st.Track == nil {
		return styleDim.Render("\n   Nothing playing. Pick a track and press enter.")
	}
	t := st.Track

	icon := "▶"
	if st.Paused {
		icon = "⏸"
	}
	repeat := styleDim.Render("repeat")
	switch {
	case st.RepeatTrack:
		repeat = styleAccent.Render("repeat one")
	case st.RepeatContext:
		repeat = styleAccent.Render("repeat")
	}
	shuffle := styleDim.Render("shuffle")
	if st.ShuffleContext {
		shuffle = styleAccent.Render("shuffle")
	}
	volume := 0
	if st.VolumeSteps > 0 {
		volume = int(st.Volume * 100 / st.VolumeSteps)
	}
	indicators := shuffle + "  " + repeat + "  " + styleDim.Render(fmt.Sprintf("vol %d%%", volume)) + " "
	title := " " + styleAccent.Render(icon) + " " + styleBold.Render(t.Name)
	if m.liked[t.Uri] {
		title += " " + styleAccent.Render("♥")
	}
	line1 := spread(title, indicators, w)

	details := strings.Join(t.ArtistNames, ", ")
	if t.AlbumName != "" {
		details += " · " + t.AlbumName
	}
	line2 := "   " + styleDim.Render(details)
	// The next track is a hint: it gets a third of the line at most, so the
	// details of the track that plays stay readable.
	if next := st.NextTrack; next != nil && next.Name != "" && w/3 >= 12 {
		hint := ansi.Truncate("next: "+next.Name, w/3, "…")
		line2 = spread(line2, styleDim.Render(hint)+" ", w)
	}

	pos, dur := t.Position, int64(t.Duration)
	left, right := formatMs(pos), formatMs(dur)
	barWidth := max(w-3-lipgloss.Width(left)-lipgloss.Width(right)-3, 0)
	filled := 0
	if dur > 0 {
		filled = int(min(max(pos, 0), dur) * int64(barWidth) / dur)
	}
	bar := styleAccent.Render(strings.Repeat("━", filled)) + styleDim.Render(strings.Repeat("─", barWidth-filled))
	line3 := "   " + left + " " + bar + " " + right

	return line1 + "\n" + line2 + "\n" + line3
}

func (m *Model) footerView(w int) string {
	if m.confirm != nil {
		return styleAccentBold.Render(fit(" "+m.confirm.question+" (y/n)", w, false))
	}
	if m.flash != "" {
		style := styleAccent
		if m.flashErr {
			style = styleError
		}
		return style.Render(fit(" "+m.flash, w, false))
	}
	hint := " space play/pause · n/p next/prev · ←/→ seek · +/- volume · s shuffle · r repeat · a actions · f filter · / search · ? help · q quit"
	if m.menu != nil {
		hint = " ↑/↓ move · enter run · esc close · ctrl+c quit"
	} else if cur := m.current(); cur.typing() {
		if _, ok := cur.(*searchPage); ok {
			hint = " typing in the search box · enter search · esc stop typing · tab switch tab · ctrl+c quit"
		} else {
			hint = " typing a filter · enter keep filter · esc clear filter · ↑/↓ move · tab switch tab · ctrl+c quit"
		}
	}
	return styleDim.Render(fit(hint, w, false))
}

func (m *Model) authView(w int) string {
	lines := []string{
		styleBold.Render("Log in to Spotify"),
		"",
		"spotui opened this page in your browser. If it did not open, visit it yourself:",
		link(m.auth.URL, w-2),
	}
	if m.auth.Code != "" {
		lines = append(lines, "", "If Spotify asks for a code, enter: "+styleAccentBold.Render(m.auth.Code))
	}
	if !m.auth.ExpiresAt.IsZero() {
		lines = append(lines, "", styleDim.Render("The code expires at "+m.auth.ExpiresAt.Local().Format("15:04")+"."))
	}
	lines = append(lines, "", styleDim.Render("You need Spotify Premium. You only log in once; spotui saves the login."))
	return indent(strings.Join(lines, "\n"))
}

func (m *Model) webLoginView(w int) string {
	lines := []string{
		styleBold.Render("Log in for search and saved albums"),
		"",
		"spotui opened this page in your browser. If it did not open, visit it yourself:",
		link(m.webLoginURL, w-2),
		"",
		styleDim.Render("Waiting for Spotify… Press esc to cancel."),
	}
	return indent(strings.Join(lines, "\n"))
}

func helpView() string {
	keys := [][2]string{
		{"space", "play or pause"},
		{"n / p", "next / previous track"},
		{"← / →", "seek 5 seconds"},
		{"+ / -", "volume up / down"},
		{"s", "shuffle on or off"},
		{"r", "repeat: off, all, one"},
		{"", ""},
		{"↑ ↓ / j k", "move"},
		{"g / G", "first / last item"},
		{"pgup / pgdown", "page up / down"},
		{"enter", "open or play"},
		{"a", "actions for the track or album"},
		{"e", "add the track to the queue"},
		{"l", "like or unlike the track"},
		{"A / I", "go to the album / artist of the track"},
		{"o", "go to the playlist, album, or artist that plays"},
		{"esc", "go back, stop typing, or clear the filter"},
		{"", ""},
		{"1 / 2 / tab", "library / search"},
		{"f", "filter the list on screen"},
		{"/", "search all of Spotify"},
		{"R", "reload library"},
		{"L", "log in to the Web API"},
		{"q", "quit"},
	}
	var b strings.Builder
	b.WriteString(styleBold.Render("Keys") + "\n\n")
	for _, k := range keys {
		b.WriteString(styleAccent.Render(fit(k[0], 16, false)) + k[1] + "\n")
	}
	b.WriteString("\n" + styleDim.Render("Press any key to close."))
	return indent(b.String())
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
