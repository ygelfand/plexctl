package view

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/search"
	"github.com/ygelfand/plexctl/internal/ui"
)

type SearchStatusView struct {
	width, height int
	theme         tint.Tint
	isIndexing    bool
	progress      search.IndexProgress
	err           error
	spinner       spinner.Model
	libraries     []plex.LibraryInfo
	progressChan  chan search.IndexProgress
}

func NewSearchStatusView(theme tint.Tint, libraries []plex.LibraryInfo) *SearchStatusView {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ui.Accent(theme))

	return &SearchStatusView{
		theme:     theme,
		spinner:   s,
		libraries: libraries,
	}
}

func (v *SearchStatusView) Init() tea.Cmd {
	return nil
}

func (v *SearchStatusView) Refresh() tea.Cmd {
	v.isIndexing = true
	v.err = nil
	v.progress = search.IndexProgress{}
	v.progressChan = make(chan search.IndexProgress, 100)
	return tea.Batch(v.spinner.Tick, v.runReindex, v.waitForProgress())
}

type reindexProgressMsg search.IndexProgress
type reindexFinishedMsg struct{ err error }

func (v *SearchStatusView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
	case ui.ThemeChangedMsg:
		v.theme = msg.Theme
		v.spinner.Style = lipgloss.NewStyle().Foreground(ui.Accent(v.theme))
	case reindexProgressMsg:
		v.progress = search.IndexProgress(msg)
		return v, v.waitForProgress()
	case reindexFinishedMsg:
		v.isIndexing = false
		v.err = msg.err
		v.progressChan = nil
		return v, nil
	case tea.KeyMsg:
		if v.isIndexing {
			return v, nil
		}
		switch msg.String() {
		case "r":
			return v, v.Refresh()
		}
	case spinner.TickMsg:
		if v.isIndexing {
			var cmd tea.Cmd
			v.spinner, cmd = v.spinner.Update(msg)
			return v, cmd
		}
	}

	return v, nil
}

func (v *SearchStatusView) runReindex() tea.Msg {
	ctx := context.Background()
	idx := search.GetIndex()
	err := idx.Reindex(ctx, v.progressChan)
	v.progressChan <- search.IndexProgress{Message: "DONE"}
	return reindexFinishedMsg{err: err}
}

func (v *SearchStatusView) waitForProgress() tea.Cmd {
	return func() tea.Msg {
		if v.progressChan == nil {
			return nil
		}
		p, ok := <-v.progressChan
		if !ok || p.Message == "DONE" {
			return nil // Let runReindex return the finished msg
		}
		return reindexProgressMsg(p)
	}
}

func (v *SearchStatusView) View() string {
	idx := search.GetIndex()

	titleStyle := ui.TitleStyle(v.theme)
	labelStyle := ui.LabelStyle(v.theme)
	valueStyle := ui.ValueStyle(v.theme)

	var lines []string
	lines = append(lines, titleStyle.Render("SEARCH INDEX STATUS"))
	lines = append(lines, "")

	lastIndexed := "Never"
	if !idx.LastIndexed.IsZero() {
		lastIndexed = idx.LastIndexed.Format("2006-01-02 15:04:05")
	}

	lines = append(lines, labelStyle.Render("Last Indexed: ")+valueStyle.Render(lastIndexed))
	lines = append(lines, labelStyle.Render("Total Entries: ")+valueStyle.Render(fmt.Sprintf("%d", len(idx.Entries))))
	lines = append(lines, "")

	lines = append(lines, titleStyle.Render("LIBRARY STATISTICS"))
	for _, lib := range v.libraries {
		lines = append(lines, labelStyle.Render(lib.Title+":")+valueStyle.Render(fmt.Sprintf("%d items", lib.Count)))
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

	return lipgloss.NewStyle().Padding(2).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (v *SearchStatusView) HelpKeys() []ui.HelpKey {
	return []ui.HelpKey{
		{Key: "r", Desc: "Reindex Library"},
	}
}
