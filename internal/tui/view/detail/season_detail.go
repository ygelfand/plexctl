package detail

import (
	"context"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type SeasonDetailView struct {
	DetailBase
	children        []components.Metadata
	episodeList     list.Model
	selectedEpisode *EpisodeDetailView
}

func NewSeasonDetailView(ratingKey string, theme tint.Tint) *SeasonDetailView {
	return &SeasonDetailView{
		DetailBase:  NewDetailBase(ratingKey, theme),
		episodeList: createBaseDetailList(),
	}
}

func (v *SeasonDetailView) Init() tea.Cmd {
	return v.fetchData
}

func (v *SeasonDetailView) Refresh() tea.Cmd {
	if v.selectedEpisode != nil {
		return v.selectedEpisode.Refresh()
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

func (v *SeasonDetailView) GetSelectedMetadata() *components.Metadata {
	if v.selectedEpisode != nil {
		return v.selectedEpisode.GetSelectedMetadata()
	}
	if v.episodeList.SelectedItem() != nil {
		meta := v.episodeList.SelectedItem().(metadataItem).metadata
		return &meta
	}
	return v.Metadata
}

func (v *SeasonDetailView) fetchData() tea.Msg {
	ctx := context.Background()
	meta, err := plex.GetMetadata(ctx, v.RatingKey, true)
	if err != nil {
		return err
	}

	children, _ := plex.GetChildren(ctx, v.RatingKey)
	return detailDataMsg{metadata: meta, children: children}
}

func (v *SeasonDetailView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v.selectedEpisode != nil {
		newEp, cmd := v.selectedEpisode.Update(msg)
		if _, ok := msg.(BackMsg); ok {
			v.selectedEpisode = nil
			return v, nil
		}
		v.selectedEpisode = newEp.(*EpisodeDetailView)
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
		v.episodeList.SetItems(items)
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if v.episodeList.SelectedItem() != nil {
				child := v.episodeList.SelectedItem().(metadataItem).metadata
				v.selectedEpisode = NewEpisodeDetailView(*child.RatingKey, v.Theme)
				return v, v.selectedEpisode.Init()
			}
		case "esc", "backspace":
			return v, func() tea.Msg { return BackMsg{} }
		case "S":
			if v.Metadata != nil && v.Metadata.ParentRatingKey != nil {
				return v, func() tea.Msg {
					return ui.JumpToDetailMsg{
						RatingKey: *v.Metadata.ParentRatingKey,
						Type:      "show",
					}
				}
			}
		}
	}

	cmd := v.DetailBase.Update(msg)
	cmds = append(cmds, cmd)

	v.episodeList.SetSize(ui.GetLayout().InnerWidth()-4, ui.GetLayout().ContentHeight()/2)

	return v, tea.Batch(cmds...)
}

func (v *SeasonDetailView) IsAtRoot() bool {
	return v.selectedEpisode == nil
}

func (v *SeasonDetailView) HelpKeys() []ui.HelpKey {
	if v.selectedEpisode != nil {
		return v.selectedEpisode.HelpKeys()
	}
	return []ui.HelpKey{
		{Key: "enter", Desc: "View Episode Details"},
		{Key: "S", Desc: "Go to Show"},
		{Key: "esc", Desc: "Back"},
		{Key: "j/up", Desc: "Move Up / Scroll"},
		{Key: "k/down", Desc: "Move Down / Scroll"},
	}
}

func (v *SeasonDetailView) View() string {
	if v.selectedEpisode != nil {
		return v.selectedEpisode.View()
	}
	if v.Loading {
		return lipgloss.NewStyle().Padding(2).Render("Loading season details...")
	}
	if v.Metadata == nil {
		return "No metadata available"
	}

	rightWidth := v.SynopsisVP.Width

	watched := RenderWatchedStatus(v.Metadata)
	headerInfo := ""
	if watched != "" {
		headerInfo = lipgloss.NewStyle().Foreground(v.Theme.BrightCyan()).MarginBottom(1).Width(rightWidth).Render(watched)
	}

	infoSection := lipgloss.JoinVertical(lipgloss.Left,
		v.RenderHeader(rightWidth),
		headerInfo,
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
		Render(v.episodeList.View())

	return lipgloss.NewStyle().Padding(1, 2).Render(lipgloss.JoinVertical(lipgloss.Left, mainLayout, listContent)) +
		"\n\n " + lipgloss.NewStyle().Foreground(v.Theme.BrightBlack()).Render("[enter] Details | [p] Play | [S] Show | [esc] Back")
}
