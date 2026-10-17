package view

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

// FriendsView keeps shared browsing separate from the user's own library tabs.
type FriendsView struct {
	theme           tint.Tint
	table           ui.Table
	servers         []components.PlexDevice
	libraries       []plex.LibraryInfo
	server          config.ServerTarget
	media           *MediaView
	requestID       uint64
	loading, loaded bool
	err             error
}

var friendsRequestID atomic.Uint64

type FriendsServersMsg struct {
	RequestID uint64
	Servers   []components.PlexDevice
	Err       error
}

type FriendsLibrariesMsg struct {
	RequestID uint64
	Server    config.ServerTarget
	Libraries []plex.LibraryInfo
	Err       error
}

func NewFriendsView(theme tint.Tint) *FriendsView {
	return &FriendsView{theme: theme, table: ui.NewTable([]table.Column{
		{Title: "SERVER / LIBRARY", Width: 35}, {Title: "SHARED BY / TYPE", Width: 30},
	}, theme)}
}

func (v *FriendsView) Init() tea.Cmd {
	if v.loaded || v.loading {
		return nil
	}
	return v.Refresh()
}

func (v *FriendsView) Refresh() tea.Cmd {
	if v.media != nil {
		return v.media.Refresh()
	}
	v.requestID = friendsRequestID.Add(1)
	v.loading, v.err = true, nil
	id := v.requestID
	server := config.Get().ServerTarget()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(server.Context(context.Background()), 15*time.Second)
		defer cancel()
		servers, err := plex.DiscoverServers(ctx)
		var shared []components.PlexDevice
		for _, server := range servers {
			if !server.Owned {
				shared = append(shared, server)
			}
		}
		return FriendsServersMsg{RequestID: id, Servers: shared, Err: err}
	}
}

func (v *FriendsView) connect(device components.PlexDevice) tea.Cmd {
	v.requestID = friendsRequestID.Add(1)
	v.loading, v.err = true, nil
	id := v.requestID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server, err := plex.ConnectServer(ctx, device)
		result := FriendsLibrariesMsg{RequestID: id, Server: server, Err: err}
		if err != nil {
			return result
		}
		ctx = server.Context(ctx)
		client, err := plex.NewClientForContext(ctx)
		if err != nil {
			result.Err = err
			return result
		}
		response, err := client.SDK.Library.GetSections(ctx)
		if err != nil {
			result.Err = fmt.Errorf("load shared libraries: %w", err)
			return result
		}
		if response.Object == nil || response.Object.MediaContainer == nil {
			result.Err = fmt.Errorf("no library data returned by %s", server.Name)
			return result
		}
		for _, dir := range response.Object.MediaContainer.Directory {
			if dir.Key != nil && dir.Title != nil {
				result.Libraries = append(result.Libraries, plex.LibraryInfo{ID: *dir.Key, Title: *dir.Title, Type: string(dir.Type)})
			}
		}
		return result
	}
}

func (v *FriendsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case FriendsServersMsg:
		if msg.RequestID != v.requestID {
			return v, nil
		}
		v.loading, v.loaded, v.err = false, msg.Err == nil, msg.Err
		if msg.Err == nil {
			v.servers, v.server, v.libraries = msg.Servers, config.ServerTarget{}, nil
			v.serverRows()
		}
		return v, nil
	case FriendsLibrariesMsg:
		if msg.RequestID != v.requestID {
			return v, nil
		}
		v.loading, v.err = false, msg.Err
		if msg.Err == nil {
			v.server, v.libraries = msg.Server, msg.Libraries
			var rows []table.Row
			for _, lib := range v.libraries {
				rows = append(rows, table.Row{lib.Title, lib.Type})
			}
			v.table.SetRows(rows)
			v.table.SetCursor(0)
		}
		return v, nil
	case tea.WindowSizeMsg:
		v.table.SetWidth(ui.GetLayout().InnerWidth())
		v.table.SetHeight(max(1, ui.GetLayout().ContentHeight()-5))
	case ui.ThemeChangedMsg:
		v.theme = msg.Theme
		ui.UpdateTableTheme(&v.table, ui.GetLayout().InnerWidth(), ui.GetLayout().TotalHeight())
	}
	if v.media != nil {
		if key, ok := msg.(tea.KeyMsg); ok && (key.String() == "esc" || key.String() == "backspace") && !v.media.details.Active() {
			v.media = nil
			return v, nil
		}
		_, cmd := v.media.Update(msg)
		return v, cmd
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		mouse.Y -= 2
		if v.table.Click(mouse) {
			msg = ui.KeyPress("enter")
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "backspace":
			v.requestID = friendsRequestID.Add(1) // Ignore a connection finishing after Back.
			v.loading, v.err, v.server, v.libraries = false, nil, config.ServerTarget{}, nil
			v.serverRows()
			return v, nil
		case "r":
			return v, v.Refresh()
		case "enter":
			if v.loading {
				return v, nil
			}
			i := v.table.Cursor()
			if v.server.ID == "" {
				if i >= 0 && i < len(v.servers) {
					return v, v.connect(v.servers[i])
				}
			} else if i >= 0 && i < len(v.libraries) {
				lib := v.libraries[i]
				v.media = NewMediaView(lib.ID, v.server.Name+" / "+lib.Title, v.theme, v.server)
				return v, v.media.Init()
			}
			return v, nil
		}
	}
	var cmd tea.Cmd
	v.table, cmd = v.table.Update(msg)
	return v, cmd
}

func (v *FriendsView) serverRows() {
	var rows []table.Row
	for _, server := range v.servers {
		rows = append(rows, table.Row{server.Name, plex.ServerDescription(server)})
	}
	v.table.SetRows(rows)
	v.table.SetCursor(0)
}

func (v *FriendsView) View() string {
	if v.media != nil {
		return v.media.View()
	}
	heading := "Friends Libraries"
	if v.server.ID != "" {
		heading = v.server.Name + " · Shared libraries"
	}
	heading = lipgloss.NewStyle().Foreground(ui.Accent(v.theme)).Bold(true).Render(heading)
	if v.loading {
		return heading + "\n\nConnecting to Plex… [esc] Back"
	}
	if v.err != nil {
		return heading + "\n\n" + v.err.Error() + "\n\n[r] Refresh · [enter] Retry selection · [esc] Back"
	}
	if len(v.servers) == 0 {
		return heading + "\n\nNo shared servers are available to this Plex user.\nAccept a library invitation in Plex, then press r to refresh."
	}
	if v.server.ID != "" && len(v.libraries) == 0 {
		return heading + "\n\nThis server has no libraries shared with this Plex user.\n[esc] Friends · [r] Refresh"
	}
	hint := "[enter] Browse shared libraries · [r] Refresh"
	if v.server.ID != "" {
		hint = "[enter] Browse library · [/] Search this server · [esc] Friends"
	}
	return heading + "\n\n" + v.table.View() + "\n" + hint
}

func (v *FriendsView) MediaServer() config.ServerTarget { return v.server }
func (v *FriendsView) ActionRows() int                  { return 1 }
func (v *FriendsView) GetSelectedMetadata() *components.Metadata {
	if v.media != nil {
		return v.media.GetSelectedMetadata()
	}
	return nil
}
func (v *FriendsView) ShowDetail(msg ui.JumpToDetailMsg) tea.Cmd {
	if msg.Server.CacheNamespace() != v.server.CacheNamespace() {
		return nil
	}
	if v.media == nil || (msg.SectionID != "" && v.media.sectionID != msg.SectionID) {
		title := "Search results"
		for _, lib := range v.libraries {
			if lib.ID == msg.SectionID {
				title = lib.Title
				break
			}
		}
		v.media = NewMediaView(msg.SectionID, v.server.Name+" / "+title, v.theme, v.server)
	}
	return v.media.ShowDetail(msg.RatingKey, msg.Type)
}
func (v *FriendsView) CanJumpLetters() bool { return v.media != nil && v.media.CanJumpLetters() }
func (v *FriendsView) AlphabetRail() *ui.AlphabetRail {
	if v.media != nil {
		return v.media.AlphabetRail()
	}
	return nil
}
func (v *FriendsView) JumpToLetter(letter string) tea.Cmd {
	if v.media != nil {
		return v.media.JumpToLetter(letter)
	}
	return nil
}
func (v *FriendsView) HelpKeys() []ui.HelpKey {
	if v.media != nil {
		return v.media.HelpKeys()
	}
	return []ui.HelpKey{{Key: "enter", Desc: "Browse shared server or library"}, {Key: "/", Desc: "Search the selected friend's server"}, {Key: "esc", Desc: "Back to friends"}, {Key: "r", Desc: "Refresh shared servers"}}
}
