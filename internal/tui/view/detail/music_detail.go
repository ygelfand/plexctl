package detail

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/paginator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/ui"
)

// MusicDetailView navigates artist -> album -> track while retaining each
// parent's selection and scroll position when returning from a child.
type MusicDetailView struct {
	DetailBase
	items      list.Model
	listRect   ui.Rect
	selected   *MusicDetailView
	albums     *poster.PosterGrid
	albumsRect ui.Rect
	rail       ui.AlphabetRail
	jumpNote   string
}

func NewMusicDetailView(ratingKey string, theme tint.Tint, servers ...config.ServerTarget) *MusicDetailView {
	items := createBaseDetailList()
	items.Paginator.Type = paginator.Arabic
	return &MusicDetailView{DetailBase: NewDetailBase(ratingKey, theme, servers...), items: items,
		albums: poster.NewPosterGrid(nil, theme, servers...)}
}

func (v *MusicDetailView) Init() tea.Cmd { return v.fetchData }

func (v *MusicDetailView) Refresh() tea.Cmd {
	if v.selected != nil {
		return v.selected.Refresh()
	}
	v.Loading = true
	return v.fetchData
}

func (v *MusicDetailView) fetchData() tea.Msg {
	ctx, cancel := context.WithTimeout(v.Server.Context(context.Background()), 30*time.Second)
	defer cancel()
	meta, err := plex.GetMetadata(ctx, v.RatingKey, true)
	if err != nil {
		return v.dataError(err)
	}
	var children []components.Metadata
	if meta.Type == "artist" || meta.Type == "album" {
		children, err = plex.GetMusicChildren(ctx, meta)
		if err != nil {
			return v.dataError(err)
		}
	}
	return detailDataMsg{serverID: v.Server.CacheNamespace(), ratingKey: v.RatingKey, metadata: meta, children: children}
}

func (v *MusicDetailView) GetSelectedMetadata() *components.Metadata {
	if v.selected != nil {
		return v.selected.GetSelectedMetadata()
	}
	if v.Metadata != nil && v.Metadata.Type == "artist" && len(v.albums.Items) > 0 {
		return &v.albums.Items[v.albums.Cursor].Metadata
	}
	if item, ok := v.items.SelectedItem().(musicItem); ok {
		return &item.metadata
	}
	return v.Metadata
}

func (v *MusicDetailView) IsAtRoot() bool { return v.selected == nil }

func (v *MusicDetailView) CanJumpLetters() bool {
	if v.selected != nil {
		return v.selected.CanJumpLetters()
	}
	return v.Metadata != nil && (v.Metadata.Type == "artist" || v.Metadata.Type == "album")
}

func (v *MusicDetailView) AlphabetRail() *ui.AlphabetRail {
	if v.selected != nil {
		return v.selected.AlphabetRail()
	}
	if v.Loading || v.Err != nil || !v.CanJumpLetters() {
		return nil
	}
	return &v.rail
}

func (v *MusicDetailView) JumpToLetter(letter string) tea.Cmd {
	if v.selected != nil {
		return v.selected.JumpToLetter(letter)
	}
	if !v.CanJumpLetters() {
		return nil
	}
	v.jumpNote = ""
	if v.Metadata.Type == "artist" {
		for i, item := range v.albums.Items {
			if ui.AlphabetLetter(item.Metadata) == letter {
				return v.albums.SetCursor(i)
			}
		}
	} else {
		for i, item := range v.items.Items() {
			if ui.AlphabetLetter(item.(musicItem).metadata) == letter {
				v.items.Select(i)
				return nil
			}
		}
	}
	v.jumpNote = "No titles under " + letter
	return nil
}

func (v *MusicDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if image, ok := msg.(PosterDataMsg); ok && image.ratingKey == v.RatingKey {
		return v, v.DetailBase.Update(image)
	}
	if posterMsg, ok := msg.(poster.PosterLoadedMsg); ok {
		v.albums.Update(posterMsg)
	}
	if v.selected != nil {
		if _, back := msg.(BackMsg); back && v.selected.IsAtRoot() {
			v.selected = nil
			return v, tea.Batch(v.albums.Init(), fetchPoster(v.Metadata, v.Width, v.Server))
		}
		_, cmd := v.selected.Update(msg)
		return v, cmd
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if letter := v.rail.LetterAt(mouse); letter != "" {
			return v, v.JumpToLetter(letter)
		}
		if v.Metadata != nil && v.Metadata.Type == "artist" {
			if local, inside := v.albumsRect.Local(mouse); inside {
				_, cmd := v.albums.Update(local)
				return v, cmd
			}
			return v, nil
		}
		if local, inside := v.listRect.Local(mouse); inside {
			if ui.ListMouse(&v.items, local) {
				return v.Update(ui.KeyPress("enter"))
			}
			return v, nil
		}
	}
	switch msg := msg.(type) {
	case detailDataMsg:
		if !v.Accepts(msg) {
			return v, nil
		}
		if msg.metadata != nil && msg.metadata.RatingKey != nil && *msg.metadata.RatingKey != v.RatingKey {
			return v, nil
		}
		if msg.metadata != nil && msg.metadata.Type == "artist" {
			slices.SortStableFunc(msg.children, func(a, b components.Metadata) int {
				return strings.Compare(ui.AlphabetTitle(a), ui.AlphabetTitle(b))
			})
		}
		items := make([]list.Item, 0, len(msg.children))
		for _, child := range msg.children {
			items = append(items, musicItem{metadataItem{metadata: child}})
		}
		v.items.SetItems(items)
		v.items.Title = "Tracks"
		if msg.metadata != nil && msg.metadata.Type == "artist" {
			v.items.Title = "Albums"
			baseCmd := v.DetailBase.Update(msg)
			posterCmd := v.albums.SetItems(msg.children)
			v.View() // Establish the embedded grid's bounds before navigation.
			return v, tea.Batch(baseCmd, posterCmd, v.albums.Init())
		}
	case poster.PosterLoadedMsg:
		_, cmd := v.albums.Update(msg)
		return v, cmd
	case poster.ItemSelectedMsg:
		if msg.Metadata.RatingKey != nil {
			v.selected = NewMusicDetailView(*msg.Metadata.RatingKey, v.Theme, v.Server)
			return v, v.selected.Init()
		}
	case tea.WindowSizeMsg:
		baseCmd := v.DetailBase.Update(msg)
		v.View()
		return v, tea.Batch(baseCmd, v.albums.Init())
	case ui.ThemeChangedMsg:
		v.albums.Theme = msg.Theme
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if meta := v.GetSelectedMetadata(); meta != nil && meta.RatingKey != nil && meta.Type != "artist" {
				v.selected = NewMusicDetailView(*meta.RatingKey, v.Theme, v.Server)
				return v, v.selected.Init()
			}
		case "esc", "backspace":
			return v, func() tea.Msg { return BackMsg{} }
		case "home":
			if v.Metadata != nil && v.Metadata.Type == "album" {
				v.items.Select(0)
				return v, nil
			}
		case "end":
			if v.Metadata != nil && v.Metadata.Type == "album" {
				v.items.Select(max(0, len(v.items.Items())-1))
				return v, nil
			}
		}
	}
	baseCmd := v.DetailBase.Update(msg)
	if v.Metadata != nil && v.Metadata.Type == "artist" {
		_, cmd := v.albums.Update(msg)
		return v, tea.Batch(baseCmd, cmd)
	}
	var listCmd tea.Cmd
	v.items, listCmd = v.items.Update(msg)
	return v, tea.Batch(baseCmd, listCmd)
}

func (v *MusicDetailView) HelpKeys() []ui.HelpKey {
	return []ui.HelpKey{
		{Key: "enter", Desc: "Open Album / Track Details"},
		{Key: "p", Desc: "Play Selected Track or Entire Album"},
		{Key: "v", Desc: "Choose Terminal Music Player / Separate Window"},
		{Key: "↑/↓", Desc: "Select Album / Track"},
		{Key: "g then letter", Desc: "Jump to an album or track title"},
		{Key: "esc", Desc: "Back"},
	}
}

func (v *MusicDetailView) View() string {
	if v.selected != nil {
		return v.selected.View()
	}
	if v.Loading {
		return "\n  Loading music..."
	}
	if v.Err != nil {
		return "\n  " + v.Err.Error() + "\n\n [esc] Back"
	}
	if v.Metadata == nil {
		return "\n  No music metadata available\n\n [esc] Back"
	}
	width := max(1, ui.GetLayout().InnerWidth()-4)
	infoWidth := width
	if v.Metadata.Type == "track" {
		infoWidth = v.SynopsisVP.Width
	}
	info := musicItem{metadataItem{metadata: *v.Metadata}}.Description()
	if v.Metadata.Type == "artist" {
		info = fmt.Sprintf("%d albums · Select a cover to browse tracks", len(v.albums.Items))
	}
	header := lipgloss.JoinVertical(lipgloss.Left,
		v.RenderHeader(infoWidth),
		lipgloss.NewStyle().Width(infoWidth).Foreground(v.Theme.BrightCyan()).Render(info),
	)
	v.listRect = ui.Rect{}
	v.rail.Rect = ui.Rect{}
	footer := "[p] Play | [v] Player | [esc] Back"
	switch v.Metadata.Type {
	case "track":
		header += "\n\n" + renderBadges(v.Metadata, v.Theme)
		header = v.RenderPosterAndInfo(header)
	case "artist":
		rect := ui.Rect{X: 2, Y: 2 + lipgloss.Height(header), Width: width}
		rect.Height = max(1, ui.GetLayout().ContentHeight()-rect.Y-3)
		v.rail.SetBounds(2+width, rect.Y, rect.Height)
		rect.Width -= v.rail.Width
		if rect != v.albumsRect {
			v.albumsRect = rect
			v.albums.SetSize(rect.Width, rect.Height)
		}
		active := ""
		if meta := v.GetSelectedMetadata(); meta != nil {
			active = ui.AlphabetLetter(*meta)
		}
		header += "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(rect.Width).MaxWidth(rect.Width).Render(v.albums.View()),
			v.rail.View(active, v.Theme))
		footer = "[g] A–Z | [p] Play | [v] Player | [esc] Back"
	default:
		v.listRect = ui.Rect{X: 2, Y: 2 + lipgloss.Height(header), Width: width}
		v.listRect.Height = max(4, ui.GetLayout().ContentHeight()-v.listRect.Y-3)
		v.rail.SetBounds(2+width, v.listRect.Y, v.listRect.Height)
		v.listRect.Width -= v.rail.Width
		coverWidth := detailPosterWidth(v.Metadata)
		cover := v.Poster
		if v.posterWidth != coverWidth || cover == "" {
			cover = lipgloss.NewStyle().Width(coverWidth).Height((coverWidth+1)/2).
				Align(lipgloss.Center, lipgloss.Center).Render("♫")
		}
		cover = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(v.Theme.BrightBlack()).Width(coverWidth).Render(cover)
		left := ""
		if v.listRect.Width >= coverWidth+2+2+18 {
			left = lipgloss.NewStyle().MarginRight(2).Render(cover)
			v.listRect.X += coverWidth + 4
			v.listRect.Width -= coverWidth + 4
		} else if v.listRect.Height >= lipgloss.Height(cover)+6 {
			// In very narrow panes the cover sits above a full-width track list.
			header += "\n\n" + cover
			v.listRect.Y += lipgloss.Height(cover) + 2
			v.listRect.Height -= lipgloss.Height(cover) + 2
			v.rail.SetBounds(2+width, v.listRect.Y, v.listRect.Height)
			v.listRect.Width = width - v.rail.Width
		}
		v.items.SetSize(max(1, v.listRect.Width), v.listRect.Height)
		active := ""
		if meta := v.GetSelectedMetadata(); meta != nil {
			active = ui.AlphabetLetter(*meta)
		}
		tracks := lipgloss.NewStyle().Width(v.listRect.Width).MaxWidth(v.listRect.Width).Render(v.items.View())
		header += "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, tracks, v.rail.View(active, v.Theme))
		footer = "[g] A–Z | [p] Play | [v] Player | [esc] Back"
	}
	if v.jumpNote != "" {
		footer = v.jumpNote + " | [esc] Back"
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(header) + "\n\n " +
		lipgloss.NewStyle().Foreground(v.Theme.BrightBlack()).Render(footer)
}

type musicItem struct{ metadataItem }

func (i musicItem) Title() string {
	if i.metadata.Type == "track" && i.metadata.Index != nil {
		return fmt.Sprintf("%02d. %s", *i.metadata.Index, i.metadata.Title)
	}
	return i.metadata.Title
}

func (i musicItem) Description() string {
	var parts []string
	if i.metadata.GrandparentTitle != nil {
		parts = append(parts, *i.metadata.GrandparentTitle)
	}
	if i.metadata.ParentTitle != nil {
		parts = append(parts, *i.metadata.ParentTitle)
	}
	if i.metadata.Type == "track" && i.metadata.Duration != nil {
		seconds := *i.metadata.Duration / 1000
		parts = append(parts, fmt.Sprintf("%d:%02d", seconds/60, seconds%60))
	} else if i.metadata.Year != nil {
		parts = append(parts, fmt.Sprint(*i.metadata.Year))
	}
	return strings.Join(parts, " · ")
}
