package view

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/search"
	"github.com/ygelfand/plexctl/internal/ui"
)

type SearchStatusView struct {
	viewport     viewport.Model
	theme        tint.Tint
	isIndexing   bool
	progress     search.IndexProgress
	err          error
	spinner      spinner.Model
	libraries    []plex.LibraryInfo
	progressChan chan search.IndexProgress
}

func NewSearchStatusView(theme tint.Tint, libraries []plex.LibraryInfo) *SearchStatusView {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ui.Accent(theme))
	vp := viewport.New(0, 0)
	vp.SetHorizontalStep(3)

	return &SearchStatusView{
		viewport:  vp,
		theme:     theme,
		spinner:   s,
		libraries: libraries,
	}
}

func (v *SearchStatusView) Init() tea.Cmd {
	return nil
}

func (v *SearchStatusView) Refresh() tea.Cmd {
	if v.isIndexing {
		return nil
	}
	v.isIndexing = true
	v.err = nil
	v.progress = search.IndexProgress{}
	v.progressChan = make(chan search.IndexProgress, 100)
	progress := v.progressChan
	index := search.GetIndex()
	return tea.Batch(v.spinner.Tick, func() tea.Msg {
		defer close(progress)
		err := index.Reindex(context.Background(), progress)
		return SearchIndexFinishedMsg{owner: v, err: err}
	}, v.waitForProgress())
}

type SearchIndexProgressMsg struct {
	owner    *SearchStatusView
	progress search.IndexProgress
}
type SearchIndexFinishedMsg struct {
	owner *SearchStatusView
	err   error
}

func (v *SearchStatusView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if mouse, ok := msg.(tea.MouseMsg); ok {
		v.updateViewport()
		local, inside := (ui.Rect{X: 2, Y: 2, Width: v.viewport.Width, Height: v.viewport.Height}).Local(mouse)
		if !inside {
			return v, nil
		}
		msg = local
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.updateViewport()
	case ui.ThemeChangedMsg:
		v.theme = msg.Theme
		v.spinner.Style = lipgloss.NewStyle().Foreground(ui.Accent(v.theme))
	case SearchIndexProgressMsg:
		if msg.owner != v || !v.isIndexing {
			return v, nil
		}
		v.progress = msg.progress
		return v, v.waitForProgress()
	case SearchIndexFinishedMsg:
		if msg.owner != v {
			return v, nil
		}
		v.isIndexing = false
		v.err = msg.err
		v.progressChan = nil
		return v, nil
	case tea.KeyMsg:
		v.updateViewport()
		switch msg.String() {
		case "r":
			return v, v.Refresh()
		case "home":
			v.viewport.GotoTop()
			return v, nil
		case "end":
			v.viewport.GotoBottom()
			return v, nil
		}
	case spinner.TickMsg:
		if v.isIndexing {
			var cmd tea.Cmd
			v.spinner, cmd = v.spinner.Update(msg)
			return v, cmd
		}
	}

	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

func (v *SearchStatusView) waitForProgress() tea.Cmd {
	progress := v.progressChan
	return func() tea.Msg {
		if progress == nil {
			return nil
		}
		p, ok := <-progress
		if !ok {
			return nil
		}
		return SearchIndexProgressMsg{owner: v, progress: p}
	}
}

func (v *SearchStatusView) View() string {
	v.updateViewport()
	return lipgloss.NewStyle().Padding(2).Render(v.viewport.View())
}

func (v *SearchStatusView) updateViewport() {
	v.viewport.Width = max(1, ui.GetLayout().InnerWidth()-4)
	v.viewport.Height = max(1, ui.GetLayout().ContentHeight()-4)
	content := v.content()
	// Keep the content at least as wide as the viewport so Bubbles' horizontal
	// offset stays nonnegative when the pane is wider than the status text.
	content = lipgloss.NewStyle().Width(max(v.viewport.Width, lipgloss.Width(content))).Render(content)
	v.viewport.SetContent(content)
	v.viewport.SetYOffset(v.viewport.YOffset)
	v.viewport.ScrollRight(0)
}

func (v *SearchStatusView) content() string {
	idx := search.GetIndex()
	entries, indexedAt := idx.Snapshot()

	titleStyle := ui.TitleStyle(v.theme)
	labelStyle := ui.LabelStyle(v.theme)
	valueStyle := ui.ValueStyle(v.theme)

	var lines []string
	lines = append(lines, titleStyle.Render("SEARCH INDEX STATUS"))
	lines = append(lines, "")

	lastIndexed := "Never"
	if !indexedAt.IsZero() {
		lastIndexed = indexedAt.Format("2006-01-02 15:04:05")
	}

	lines = append(lines, labelStyle.Render("Last Indexed: ")+valueStyle.Render(lastIndexed))
	lines = append(lines, labelStyle.Render("Total Entries: ")+valueStyle.Render(fmt.Sprintf("%d", len(entries))))
	lines = append(lines, "")

	lines = append(lines, titleStyle.Render("LIBRARY STATISTICS"))
	for _, lib := range v.libraries {
		label := labelStyle.Width(max(20, lipgloss.Width(lib.Title)+2)).Render(lib.Title + ":")
		lines = append(lines, label+valueStyle.Render(fmt.Sprintf("%d items", lib.Count)))
	}
	lines = append(lines, "")

	if v.isIndexing {
		progText := fmt.Sprintf("[%d/%d] %s", v.progress.Current, v.progress.Total, v.progress.Message)
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left,
			v.spinner.View(),
			" ",
			ui.AccentStyle(v.theme).Render("Indexing... "),
			lipgloss.NewStyle().Foreground(v.theme.White()).Render(progText),
		))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(v.theme.BrightBlack()).Render("Press 'r' to reindex entire library"))
	}

	if v.err != nil {
		lines = append(lines, "")
		lines = append(lines, ui.ErrorStyle(v.theme).Render(fmt.Sprintf("Error: %v", v.err)))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (v *SearchStatusView) HelpKeys() []ui.HelpKey {
	return []ui.HelpKey{
		{Key: "j/down, k/up", Desc: "Scroll Down / Up"},
		{Key: "h/left, l/right", Desc: "Scroll Left / Right"},
		{Key: "r", Desc: "Reindex Library"},
	}
}
