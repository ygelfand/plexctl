package detail

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/player"
	"github.com/ygelfand/plexctl/internal/ui"
)

type MovieDetailView struct {
	DetailBase
}

func NewMovieDetailView(ratingKey string, theme tint.Tint) *MovieDetailView {
	return &MovieDetailView{
		DetailBase: NewDetailBase(ratingKey, theme),
	}
}

func (v *MovieDetailView) Init() tea.Cmd {
	return v.fetchData
}

func (v *MovieDetailView) Refresh() tea.Cmd {
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

func (v *MovieDetailView) GetSelectedMetadata() *components.Metadata {
	return v.Metadata
}

func (v *MovieDetailView) fetchData() tea.Msg {
	ctx := context.Background()
	meta, err := plex.GetMetadata(ctx, v.RatingKey, false)
	if err != nil {
		return err
	}
	return detailDataMsg{metadata: meta}
}

func (v *MovieDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "t":
			if v.hasTrailer() {
				return v, v.playTrailer()
			}
		case "esc", "backspace":
			return v, func() tea.Msg { return BackMsg{} }
		}
	}

	cmd := v.DetailBase.Update(msg)
	cmds = append(cmds, cmd)

	return v, tea.Batch(cmds...)
}

func (v *MovieDetailView) playTrailer() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		targetKey := ""
		if v.Metadata.PrimaryExtraKey != nil {
			targetKey = *v.Metadata.PrimaryExtraKey
		} else if v.Metadata.Extras != nil {
			for _, extra := range v.Metadata.Extras.Metadata {
				if extra.Subtype != nil && *extra.Subtype == "trailer" {
					slog.Debug("Found trailer", "extra", extra)
					targetKey = *extra.RatingKey
					break
				}
			}
		}

		if targetKey == "" {
			return fmt.Errorf("no trailer found")
		}
		parts := strings.Split(targetKey, "/")
		targetKey = parts[len(parts)-1]
		meta, err := plex.GetMetadata(ctx, targetKey, false)
		if err != nil {
			return err
		}
		return player.PlayMedia(meta, true, false, 0)()
	}
}

func (v *MovieDetailView) hasTrailer() bool {
	if v.Metadata == nil {
		return false
	}
	if v.Metadata.PrimaryExtraKey != nil {
		return true
	}
	if v.Metadata.Extras != nil {
		for _, extra := range v.Metadata.Extras.Metadata {
			if extra.Subtype != nil && *extra.Subtype == "trailer" {
				return true
			}
		}
	}
	return false
}

func (v *MovieDetailView) HelpKeys() []ui.HelpKey {
	keys := []ui.HelpKey{
		{Key: "esc", Desc: "Back"},
		{Key: "j/up", Desc: "Scroll Synopsis Up"},
		{Key: "k/down", Desc: "Scroll Synopsis Down"},
	}
	if v.hasTrailer() {
		keys = append(keys, ui.HelpKey{Key: "t", Desc: "Watch Trailer"})
	}
	return keys
}

func (v *MovieDetailView) View() string {
	if v.Loading {
		return lipgloss.NewStyle().Padding(2).Render("Loading movie details...")
	}
	if v.Metadata == nil {
		return "No metadata available"
	}

	rightWidth := v.SynopsisVP.Width
	labelStyle := lipgloss.NewStyle().Foreground(v.Theme.BrightBlack()).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(v.Theme.White())

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
	if v.Metadata.Rating != nil {
		headerParts = append(headerParts, fmt.Sprintf("★ %.1f", *v.Metadata.Rating))
	}
	watched := RenderWatchedStatus(v.Metadata)
	if watched != "" {
		headerParts = append(headerParts, watched)
	}
	headerInfo := strings.Join(headerParts, "  •  ")

	// Extra Info
	var subtitleRows []string
	if v.Metadata.Tagline != nil && *v.Metadata.Tagline != "" {
		subtitleRows = append(subtitleRows, lipgloss.NewStyle().Italic(true).Foreground(v.Theme.BrightCyan()).Width(rightWidth).Render(*v.Metadata.Tagline))
	}

	var directors []string
	for _, d := range v.Metadata.Director {
		directors = append(directors, d.Tag)
	}
	directorStr := "Unknown"
	if len(directors) > 0 {
		directorStr = strings.Join(directors, ", ")
	}

	var genres []string
	for _, g := range v.Metadata.Genre {
		genres = append(genres, g.Tag)
	}
	genreStr := strings.Join(genres, ", ")

	var cast []string
	for i, r := range v.Metadata.Role {
		if i > 4 { // Limit cast
			break
		}
		cast = append(cast, r.Tag)
	}
	castStr := strings.Join(cast, ", ")

	detailsSection := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Width(rightWidth).Render(lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("DIRECTED BY: "), valueStyle.Render(directorStr))),
		lipgloss.NewStyle().Width(rightWidth).Render(lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("GENRE:       "), valueStyle.Render(genreStr))),
		lipgloss.NewStyle().Width(rightWidth).Render(lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("CAST:        "), valueStyle.Render(castStr))),
	)

	infoSection := lipgloss.JoinVertical(lipgloss.Left,
		v.RenderHeader(rightWidth),
		lipgloss.NewStyle().Foreground(v.Theme.BrightCyan()).MarginBottom(1).Width(rightWidth).Render(headerInfo),
		lipgloss.JoinVertical(lipgloss.Left, subtitleRows...),
		"",
		detailsSection,
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

	footer := "[p] Play "
	if v.hasTrailer() {
		footer += "| [t] Trailer "
	}
	footer += "| [esc] Back | [↑/↓] Scroll Synopsis"

	return lipgloss.NewStyle().Padding(1, 2).Render(mainLayout) +
		"\n\n " + lipgloss.NewStyle().Foreground(v.Theme.BrightBlack()).Render(footer)
}
