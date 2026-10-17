package detail

import (
	"fmt"
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

type DetailBase struct {
	Server      config.ServerTarget
	RatingKey   string
	Metadata    *components.Metadata
	Poster      string
	posterWidth int
	Width       int
	Height      int
	Theme       tint.Tint
	Err         error
	Loading     bool

	SynopsisVP viewport.Model
}

func NewDetailBase(ratingKey string, theme tint.Tint, servers ...config.ServerTarget) DetailBase {
	return DetailBase{Server: config.MediaServer(servers...),
		RatingKey:  ratingKey,
		Theme:      theme,
		Loading:    true,
		SynopsisVP: createBaseViewport(),
	}
}

func (b *DetailBase) SetSize(width, height int) {
	b.Width = width
	b.Height = height
	b.UpdateLayout()
}

func (b *DetailBase) IsAtRoot() bool {
	return true
}

func (b *DetailBase) Update(msg tea.Msg) tea.Cmd {
	if !b.Accepts(msg) {
		return nil
	}
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.Width = msg.Width
		b.Height = msg.Height
		b.UpdateLayout()
		if b.Metadata != nil {
			cmds = append(cmds, fetchPoster(b.Metadata, b.Width, b.Server))
		}
	case interface{ GetTheme() tint.Tint }:
		b.Theme = msg.GetTheme()
	case detailDataMsg:
		b.Loading = false
		b.Err = msg.err
		b.Metadata = msg.metadata
		b.UpdateLayout()
		cmds = append(cmds, fetchPoster(b.Metadata, b.Width, b.Server))
	case PosterDataMsg:
		if msg.ratingKey != b.RatingKey || msg.width != detailPosterWidth(b.Metadata) {
			return nil
		}
		b.Poster = msg.view
		b.posterWidth = msg.width
		b.UpdateLayout()
	case error:
		b.Loading = false
		b.Err = msg
	}

	var cmd tea.Cmd
	b.SynopsisVP, cmd = b.SynopsisVP.Update(msg)
	cmds = append(cmds, cmd)

	return tea.Batch(cmds...)
}

func (b *DetailBase) UpdateLayout() {
	layout := ui.GetLayout()
	rightWidth := layout.DetailRightColumnWidth(b.showPoster())
	b.SynopsisVP.Width = rightWidth

	// Default height for synopsis, can be overridden by specific views
	if b.Metadata != nil && b.Metadata.Type == "movie" {
		b.SynopsisVP.Height = max(layout.ContentHeight()-25, 5)
	} else {
		b.SynopsisVP.Height = 8
	}

	if b.Metadata != nil && b.Metadata.Summary != nil {
		wrapped := lipgloss.NewStyle().Width(rightWidth).Render(*b.Metadata.Summary)
		b.SynopsisVP.SetContent(wrapped)
	}
}

func (b *DetailBase) RenderHeader(rightWidth int) string {
	titleStyle := lipgloss.NewStyle().Foreground(ui.Accent(b.Theme)).Bold(true).Width(rightWidth)

	title := b.Metadata.Title
	if b.Metadata.ParentTitle != nil && (b.Metadata.Type == "season" || b.Metadata.Type == "episode") {
		title = *b.Metadata.ParentTitle + " - " + title
	}
	if b.Metadata.GrandparentTitle != nil && b.Metadata.Type == "episode" {
		title = *b.Metadata.GrandparentTitle + " - " + title
	}
	if b.Server.Shared {
		title = "Shared · " + b.Server.Name + " · " + title
	}

	return titleStyle.Render(title)
}

func (b *DetailBase) RenderPosterAndInfo(infoSection string) string {
	if b.showPoster() {
		posterBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(b.Theme.BrightBlack()).
			Render(b.Poster)

		return lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().MarginRight(ui.DetailColumnGap).Render(posterBox),
			infoSection,
		)
	}
	return infoSection
}

func (b *DetailBase) showPoster() bool {
	layout := ui.GetLayout()
	return b.Poster != "" && layout.InnerWidth() >= 70 && layout.ContentHeight() >= 30
}

func (b *DetailBase) Accepts(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case DataMsg:
		return (msg.serverID == "" || msg.serverID == b.Server.CacheNamespace()) && (msg.ratingKey == "" || msg.ratingKey == b.RatingKey)
	case PosterDataMsg:
		return msg.serverID == "" || msg.serverID == b.Server.CacheNamespace()
	}
	return true
}

func (b *DetailBase) dataError(err error) DataMsg {
	return DataMsg{serverID: b.Server.CacheNamespace(), ratingKey: b.RatingKey, err: err}
}

func (b *DetailBase) errorView() string {
	return fmt.Sprintf("Unable to load details: %v\n[r] Retry · [esc] Back", b.Err)
}
