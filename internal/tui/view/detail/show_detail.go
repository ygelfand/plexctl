package detail

import (
	"context"
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type ShowDetailView struct {
	DetailBase
	children       []components.Metadata
	seasonList     list.Model
	selectedSeason *SeasonDetailView
}

func NewShowDetailView(ratingKey string, theme tint.Tint) *ShowDetailView {
	return &ShowDetailView{
		DetailBase: NewDetailBase(ratingKey, theme),
		seasonList: createBaseDetailList(),
	}
}

func (v *ShowDetailView) Init() tea.Cmd {
	return v.fetchData
}

func (v *ShowDetailView) Refresh() tea.Cmd {
	if v.selectedSeason != nil {
		return v.selectedSeason.Refresh()
	}
	v.Loading = true
	return func() tea.Msg {
		ctx := context.Background()
		meta, err := plex.GetMetadata(ctx, v.RatingKey, true)
		if err != nil {
			return err
		}
		children, _ := plex.GetChildren(ctx, v.RatingKey)
		return detailDataMsg{metadata: meta, children: children}
	}
}

func (v *ShowDetailView) GetSelectedMetadata() *components.Metadata {
	if v.selectedSeason != nil {
		return v.selectedSeason.GetSelectedMetadata()
	}
	if v.seasonList.SelectedItem() != nil {
		meta := v.seasonList.SelectedItem().(metadataItem).metadata
		return &meta
	}
	return v.Metadata
}

func (v *ShowDetailView) fetchData() tea.Msg {
	ctx := context.Background()
	meta, err := plex.GetMetadata(ctx, v.RatingKey, false)
	if err != nil {
		return err
	}

	children, _ := plex.GetChildren(ctx, v.RatingKey)
	return detailDataMsg{metadata: meta, children: children}
}

func (v *ShowDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v.selectedSeason != nil {
		newSeason, cmd := v.selectedSeason.Update(msg)
		if _, ok := msg.(BackMsg); ok {
			v.selectedSeason = nil
			return v, nil
		}
		v.selectedSeason = newSeason.(*SeasonDetailView)
		return v, cmd
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case detailDataMsg:
		v.children = msg.children
		var items []list.Item
		for _, child := range v.children {
			items = append(items, metadataItem{metadata: child})
		}
		v.seasonList.SetItems(items)
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if v.seasonList.SelectedItem() != nil {
				child := v.seasonList.SelectedItem().(metadataItem).metadata
				v.selectedSeason = NewSeasonDetailView(*child.RatingKey, v.Theme)
				return v, v.selectedSeason.Init()
			}
		case "esc", "backspace":
			return v, func() tea.Msg { return BackMsg{} }
		}
	}

	cmd := v.DetailBase.Update(msg)
	cmds = append(cmds, cmd)

	var listCmd tea.Cmd
	v.seasonList, listCmd = v.seasonList.Update(msg)
	cmds = append(cmds, listCmd)

	v.seasonList.SetSize(ui.GetLayout().InnerWidth()-4, ui.GetLayout().ContentHeight()/2)

	return v, tea.Batch(cmds...)
}

func (v *ShowDetailView) IsAtRoot() bool {
	return v.selectedSeason == nil
}

func (v *ShowDetailView) HelpKeys() []ui.HelpKey {
	if v.selectedSeason != nil {
		return v.selectedSeason.HelpKeys()
	}
	return []ui.HelpKey{
		{Key: "enter", Desc: "Select Season"},
		{Key: "esc", Desc: "Back"},
		{Key: "j/up", Desc: "Move Up / Scroll"},
		{Key: "k/down", Desc: "Move Down / Scroll"},
	}
}

func (v *ShowDetailView) View() string {
	if v.selectedSeason != nil {
		return v.selectedSeason.View()
	}
	if v.Loading {
		return lipgloss.NewStyle().Padding(2).Render("Loading show details...")
	}
	if v.Metadata == nil {
		return "No metadata available"
	}

	rightWidth := v.SynopsisVP.Width

	var headerParts []string
	if v.Metadata.Year != nil {
		headerParts = append(headerParts, fmt.Sprintf("%d", *v.Metadata.Year))
	}
	if v.Metadata.ChildCount != nil {
		headerParts = append(headerParts, fmt.Sprintf("%d Seasons", *v.Metadata.ChildCount))
	}
	if v.Metadata.ContentRating != nil {
		headerParts = append(headerParts, *v.Metadata.ContentRating)
	}
	watched := RenderWatchedStatus(v.Metadata)
	if watched != "" {
		headerParts = append(headerParts, watched)
	}
	headerInfo := strings.Join(headerParts, "  •  ")

	var genres []string
	for _, g := range v.Metadata.Genre {
		genres = append(genres, g.Tag)
	}
	genreStr := strings.Join(genres, ", ")

	var cast []string
	for i, r := range v.Metadata.Role {
		if i > 4 {
			break
		}
		cast = append(cast, r.Tag)
	}
	castStr := strings.Join(cast, ", ")

	labelStyle := lipgloss.NewStyle().Foreground(v.Theme.BrightBlack()).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(v.Theme.White())

	detailsSection := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Width(rightWidth).Render(lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("GENRE: "), valueStyle.Render(genreStr))),
		lipgloss.NewStyle().Width(rightWidth).Render(lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render("CAST:  "), valueStyle.Render(castStr))),
	)

	infoSection := lipgloss.JoinVertical(lipgloss.Left,
		v.RenderHeader(rightWidth),
		lipgloss.NewStyle().Foreground(v.Theme.BrightCyan()).MarginBottom(1).Width(rightWidth).Render(headerInfo),
		detailsSection,
		"",
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder(), true, false, true, false).
			BorderForeground(v.Theme.BrightBlack()).
			Padding(1, 0).
			Width(rightWidth).
			Render(v.SynopsisVP.View()),
	)

	mainLayout := v.RenderPosterAndInfo(infoSection)

	listContent := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(v.Theme.BrightBlack()).
		PaddingTop(1).
		Render(v.seasonList.View())

	return lipgloss.NewStyle().Padding(1, 2).Render(lipgloss.JoinVertical(lipgloss.Left, mainLayout, listContent)) +
		"\n\n " + lipgloss.NewStyle().Foreground(v.Theme.BrightBlack()).Render("[enter] Select Season | [esc] Back")
}
