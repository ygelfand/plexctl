package view

import (
	"context"
	"fmt"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type HistoryTab struct {
	table  table.Model
	width  int
	height int

	metadata  []operations.ListPlaybackHistoryMetadata
	dayCursor time.Time
	isLoading bool
}

func NewHistoryTab(theme tint.Tint) *HistoryTab {
	columns := []table.Column{
		{Title: "DATE", Width: 18},
		{Title: "USER", Width: 12},
		{Title: "TITLE", Width: 35},
		{Title: "TYPE", Width: 8},
		{Title: "DEVICE", Width: 12},
		{Title: "LIBRARY", Width: 12},
	}

	return &HistoryTab{
		table:     ui.NewTable(columns, theme),
		dayCursor: time.Now(),
	}
}

func (t *HistoryTab) Init() tea.Cmd {
	t.isLoading = true
	return t.fetchNextPage
}

func (t *HistoryTab) Refresh() tea.Cmd {
	t.dayCursor = time.Now()
	t.metadata = nil
	t.table.SetRows(nil)
	t.isLoading = true
	return t.fetchNextPage
}

func (t *HistoryTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		t.table.SetHeight(ui.GetTableHeight(t.height))
		t.table.SetWidth(t.width)
	case ui.ThemeChangedMsg:
		ui.UpdateTableTheme(&t.table, t.width, t.height)
	case historyResult:
		t.isLoading = false
		t.metadata = append(t.metadata, msg.metadata...)
		newRows := append(t.table.Rows(), msg.rows...)
		t.table.SetRows(newRows)

		// If we still don't have enough rows to fill the table height, get more
		tableHeight := ui.GetTableHeight(t.height)
		if len(newRows) < tableHeight && len(msg.rows) > 0 && t.height > 0 {
			t.isLoading = true
			return t, t.fetchNextPage
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			idx := t.table.Cursor()
			if idx >= 0 && idx < len(t.metadata) {
				meta := t.metadata[idx]
				if meta.RatingKey != nil && meta.Type != nil {
					sectionID := ""
					if meta.LibrarySectionID != nil {
						sectionID = *meta.LibrarySectionID
					}
					return t, func() tea.Msg {
						return ui.SelectMediaMsg{
							RatingKey: *meta.RatingKey,
							Type:      *meta.Type,
							SectionID: sectionID,
						}
					}
				}
			}
		case "r":
			return t, t.Refresh()
		}
	}

	t.table, cmd = t.table.Update(msg)

	// Lazy load
	if !t.isLoading && t.table.Cursor() > len(t.table.Rows())-10 && len(t.table.Rows()) > 0 {
		t.isLoading = true
		return t, tea.Batch(cmd, t.fetchNextPage)
	}

	return t, cmd
}

type historyResult struct {
	rows     []table.Row
	metadata []operations.ListPlaybackHistoryMetadata
}

func (t *HistoryTab) fetchNextPage() tea.Msg {
	store, err := plex.GetStore()
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Try fetching days until we find data or hit a limit (e.g. 30 empty days)
	var allRows []table.Row
	var allMeta []operations.ListPlaybackHistoryMetadata

	for i := 0; i < 30; i++ {
		gte := t.dayCursor.Add(-24 * time.Hour)
		lte := t.dayCursor

		res, err := store.ListPlaybackHistory(ctx, operations.ListPlaybackHistoryRequest{
			Sort:        []string{"viewedAt:desc"},
			ViewedAtGte: ui.Ptr(gte.Unix()),
			ViewedAtLte: ui.Ptr(lte.Unix()),
		})

		t.dayCursor = gte

		if err == nil && res.Object != nil && res.Object.MediaContainer != nil {
			for _, meta := range res.Object.MediaContainer.Metadata {
				viewedAt := ""
				if meta.ViewedAt != nil {
					viewedAt = time.Unix(*meta.ViewedAt, 0).Format("2006-01-02 15:04")
				}

				user, _ := store.ResolveUser(ctx, *meta.AccountID)
				device, _ := store.ResolveDevice(ctx, fmt.Sprintf("%d", *meta.DeviceID))
				library, _ := store.ResolveLibrary(ctx, *meta.LibrarySectionID)

				title := ""
				if meta.Title != nil {
					title = *meta.Title
				}

				mType := ""
				if meta.Type != nil {
					mType = *meta.Type
				}

				allRows = append(allRows, table.Row{
					viewedAt,
					user,
					title,
					mType,
					device,
					library,
				})
				allMeta = append(allMeta, meta)
			}
		}

		if len(allRows) > 0 {
			break
		}
	}

	return historyResult{
		rows:     allRows,
		metadata: allMeta,
	}
}

func (t *HistoryTab) View() string {
	if len(t.table.Rows()) == 0 {
		return lipgloss.NewStyle().
			Padding(2).
			Foreground(ui.GetLayout().Theme().BrightBlack()).
			Render("No playback history found.")
	}
	return t.table.View()
}

func (t *HistoryTab) HelpKeys() []ui.HelpKey {
	return []ui.HelpKey{
		{Key: "j/up", Desc: "Move Up"},
		{Key: "k/down", Desc: "Move Down"},
	}
}
