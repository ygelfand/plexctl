package view

import (
	"fmt"
	"testing"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestHistoryTableNavigationPreservesActivation(t *testing.T) {
	tab := NewHistoryTab(ui.PlexctlTheme)
	rows := navigationRows(len(tab.table.Columns()))
	metadata := make([]operations.ListPlaybackHistoryMetadata, len(rows))
	for i := range metadata {
		metadata[i] = operations.ListPlaybackHistoryMetadata{RatingKey: ui.Ptr(fmt.Sprint(i)), Type: ui.Ptr("movie")}
	}
	tab.Update(DataLoadedMsg{owner: tab, data: historyResult{rows: rows, metadata: metadata}})
	verifyTableNavigation(t, tab, &tab.table)
	_, cmd := tab.Update(ui.KeyPress("enter"))
	if cmd == nil {
		t.Fatal("Enter did not activate the selected history row")
	}
	msg, ok := cmd().(ui.SelectMediaMsg)
	if !ok || msg.RatingKey != "19" {
		t.Fatalf("Enter did not open the navigated history item: %+v", msg)
	}
}
