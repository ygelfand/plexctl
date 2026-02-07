package detail

import (
	"context"
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type EpisodeDetailView struct {
	DetailBase
}

func NewEpisodeDetailView(ratingKey string, theme tint.Tint) *EpisodeDetailView {
	return &EpisodeDetailView{
		DetailBase: NewDetailBase(ratingKey, theme),
	}
}

func (v *EpisodeDetailView) Init() tea.Cmd {
	return v.fetchData
}

func (v *EpisodeDetailView) Refresh() tea.Cmd {
	v.Loading = true
	return func() tea.Msg {
		ctx := context.Background()
		meta, err := plex.GetMetadata(ctx, v.RatingKey, true)
		if err != nil {
			return err
		}
		return detailDataMsg{metadata: meta}
	}
}

func (v *EpisodeDetailView) GetSelectedMetadata() *components.Metadata {
	return v.Metadata
}

func (v *EpisodeDetailView) fetchData() tea.Msg {
	ctx := context.Background()
	meta, err := plex.GetMetadata(ctx, v.RatingKey, false)
	if err != nil {
		return err
	}
	return detailDataMsg{metadata: meta}
}

func (v *EpisodeDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "backspace":
			return v, func() tea.Msg { return BackMsg{} }
		}
	}

	cmd := v.DetailBase.Update(msg)
	cmds = append(cmds, cmd)

	return v, tea.Batch(cmds...)
}

func (v *EpisodeDetailView) HelpKeys() []ui.HelpKey {
	return []ui.HelpKey{
		{Key: "esc", Desc: "Back"},
		{Key: "j/up", Desc: "Scroll Synopsis Up"},
		{Key: "k/down", Desc: "Scroll Synopsis Down"},
	}
}

func (v *EpisodeDetailView) View() string {
	if v.Loading {
		return lipgloss.NewStyle().Padding(2).Render("Loading episode details...")
	}
	if v.Metadata == nil {
		return "No metadata available"
	}

	rightWidth := v.SynopsisVP.Width
	dimStyle := lipgloss.NewStyle().Foreground(v.Theme.BrightBlack())

	var headerParts []string
	if v.Metadata.Year != nil {
		headerParts = append(headerParts, fmt.Sprintf("%d", *v.Metadata.Year))
	}
	if v.Metadata.Duration != nil {
		headerParts = append(headerParts, ui.FormatDuration(*v.Metadata.Duration))
	}
	if v.Metadata.ContentRating != nil {
		headerParts = append(headerParts, *v.Metadata.ContentRating)
	}
	watched := RenderWatchedStatus(v.Metadata)
	if watched != "" {
		headerParts = append(headerParts, watched)
	}
	headerInfo := strings.Join(headerParts, "  •  ")

	infoSection := lipgloss.JoinVertical(lipgloss.Left,
		v.RenderHeader(rightWidth),
		lipgloss.NewStyle().Foreground(v.Theme.BrightCyan()).MarginBottom(1).Width(rightWidth).Render(headerInfo),
		"",
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder(), true, false, true, false).
			BorderForeground(v.Theme.BrightBlack()).
			Padding(1, 0).
			Width(rightWidth).
			Render(v.SynopsisVP.View()),
		"",
		lipgloss.NewStyle().Width(rightWidth).Render(renderBadges(v.Metadata, v.Theme)),
	)

	mainLayout := v.RenderPosterAndInfo(infoSection)

	return lipgloss.NewStyle().Padding(1, 2).Render(mainLayout) +
		"\n\n " + dimStyle.Render("[p] Play | [esc] Back | [↑/↓] Scroll Synopsis")
}
