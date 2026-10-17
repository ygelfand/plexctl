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
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

type HistoryTab struct {
	table  ui.Table
	width  int
	height int

	metadata  []operations.ListPlaybackHistoryMetadata
	dayCursor time.Time
	isLoading bool
	hasMore   bool
	request   viewRequest
	err       error
}

func NewHistoryTab(theme tint.Tint) *HistoryTab {
	columns := []table.Column{
		{Title: "DATE", Width: 18},
		{Title: "USER", Width: 12},
		{Title: "TITLE", Width: 50},
		{Title: "TYPE", Width: 8},
		{Title: "DEVICE", Width: 12},
		{Title: "LIBRARY", Width: 12},
	}

	return &HistoryTab{
		table:     ui.NewTable(columns, theme),
		dayCursor: time.Now(),
		hasMore:   true,
		request:   newViewRequest(),
	}
}

func (t *HistoryTab) Init() tea.Cmd {
	if t.isLoading || len(t.metadata) > 0 || !t.hasMore {
		return nil
	}
	return t.fetchNextPage()
}

func (t *HistoryTab) Refresh() tea.Cmd {
	t.dayCursor = time.Now()
	t.metadata = nil
	t.table.SetRows(nil)
	t.hasMore, t.err = true, nil
	return t.fetchNextPage()
}

func (t *HistoryTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if mouse, ok := msg.(tea.MouseMsg); ok && t.table.Click(mouse) {
		msg = ui.KeyPress("enter")
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = ui.GetLayout().InnerWidth()
		t.height = msg.Height
		t.table.SetHeight(ui.GetTableHeight(t.height))
		t.table.SetWidth(t.width)
	case ui.ThemeChangedMsg:
		ui.UpdateTableTheme(&t.table, t.width, t.height)
	case DataLoadedMsg:
		if !t.request.accepts(t, msg) {
			return t, nil
		}
		t.isLoading = false
		t.err = msg.err
		if msg.err != nil {
			return t, nil
		}
		result := msg.data.(historyResult)
		t.dayCursor = result.nextDay
		t.hasMore = len(result.rows) > 0
		t.metadata = append(t.metadata, result.metadata...)
		newRows := append(t.table.Rows(), result.rows...)
		t.table.SetRows(newRows)

		// If we still don't have enough rows to fill the table height, get more
		tableHeight := ui.GetTableHeight(t.height)
		if len(newRows) < tableHeight && t.hasMore && t.height > 0 {
			return t, t.fetchNextPage()
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
							Server:    t.request.server,
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
	if !t.isLoading && t.hasMore && t.err == nil && t.table.Cursor() > len(t.table.Rows())-10 && len(t.table.Rows()) > 0 {
		return t, tea.Batch(cmd, t.fetchNextPage())
	}

	return t, cmd
}

type historyResult struct {
	rows     []table.Row
	metadata []operations.ListPlaybackHistoryMetadata
	nextDay  time.Time
}

func (t *HistoryTab) fetchNextPage() tea.Cmd {
	t.isLoading = true
	cursor := t.dayCursor
	return t.request.start(t, func(ctx context.Context) (any, error) {
		return fetchHistory(ctx, cursor)
	})
}

func fetchHistory(ctx context.Context, cursor time.Time) (any, error) {
	store, err := plex.GetStoreForContext(ctx)
	if err != nil {
		return nil, err
	}

	// Try fetching days until we find data or hit a limit (e.g. 30 empty days)
	var allRows []table.Row
	var allMeta []operations.ListPlaybackHistoryMetadata

	for range 30 {
		gte := cursor.Add(-24 * time.Hour)
		lte := cursor

		res, err := store.ListPlaybackHistory(ctx, operations.ListPlaybackHistoryRequest{
			Sort:        []string{"viewedAt:desc"},
			ViewedAtGte: ui.Ptr(gte.Unix()),
			ViewedAtLte: ui.Ptr(lte.Unix() - 1),
		})

		if err != nil {
			return nil, err
		}
		cursor = gte

		if res.Object != nil && res.Object.MediaContainer != nil {
			userMap := make(map[int64]string)
			libMap := make(map[string]string)
			deviceMap := make(map[string]string)

			// Pre-resolve metadata for the presenter
			for _, meta := range res.Object.MediaContainer.Metadata {
				if meta.AccountID != nil {
					userMap[*meta.AccountID], _ = store.ResolveUser(ctx, *meta.AccountID)
				}
				if meta.DeviceID != nil {
					dID := fmt.Sprintf("%d", *meta.DeviceID)
					deviceMap[dID], _ = store.ResolveDevice(ctx, dID)
				}
				if meta.LibrarySectionID != nil {
					libMap[*meta.LibrarySectionID], _ = store.ResolveLibrary(ctx, *meta.LibrarySectionID)
				}
			}

			items := presenters.MapHistoryMetadata(res.Object.MediaContainer.Metadata, userMap, libMap, deviceMap)
			for i, item := range items {
				allRows = append(allRows, item.ToRow())
				allMeta = append(allMeta, res.Object.MediaContainer.Metadata[i])
			}
		}

		if len(allRows) > 0 {
			break
		}
	}

	return historyResult{
		rows:     allRows,
		metadata: allMeta,
		nextDay:  cursor,
	}, nil
}

func (t *HistoryTab) View() string {
	if t.err != nil {
		return fmt.Sprintf("Unable to load history: %v\n[r] Retry", t.err)
	}
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
		{Key: "j/down", Desc: "Next Row"},
		{Key: "k/up", Desc: "Previous Row"},
		{Key: "h/left, l/right", Desc: "Scroll Columns Left / Right"},
	}
}
