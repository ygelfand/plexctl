package detail

import (
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/ui"
)

type DetailBase struct {
	RatingKey string
	Metadata  *components.Metadata
	Poster    string
	Width     int
	Height    int
	Theme     tint.Tint
	Err       error
	Loading   bool

	SynopsisVP viewport.Model
}

func NewDetailBase(ratingKey string, theme tint.Tint) DetailBase {
	return DetailBase{
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
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.Width = msg.Width
		b.Height = msg.Height
		b.UpdateLayout()
		if b.Metadata != nil {
			cmds = append(cmds, fetchPoster(b.Metadata, b.Width))
		}
	case interface{ GetTheme() tint.Tint }:
		b.Theme = msg.GetTheme()
	case detailDataMsg:
		b.Loading = false
		b.Metadata = msg.metadata
		b.UpdateLayout()
		cmds = append(cmds, fetchPoster(b.Metadata, b.Width))
	case posterDataMsg:
		b.Poster = string(msg)
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
	rightWidth := layout.DetailRightColumnWidth(b.Poster != "")
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

	return titleStyle.Render(title)
}

func (b *DetailBase) RenderPosterAndInfo(infoSection string) string {
	if b.Poster != "" {
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
