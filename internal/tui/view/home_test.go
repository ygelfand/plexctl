package view

import (
	"fmt"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestHomeVerticalNavigationFollowsVisibleColumn(t *testing.T) {
	for _, tt := range []struct {
		name         string
		key          string
		from         int
		sourceCursor int
		sourceOffset int
		targetCursor int
		targetOffset int
		targetCount  int
		want         int
	}{
		{name: "down ignores previous selection", key: "j", sourceCursor: 2, targetCount: 12, want: 2},
		{name: "up ignores previous selection", key: "k", from: 1, sourceCursor: 1, targetCursor: 3, targetCount: 12, want: 1},
		{name: "down arrow into scrolled row", key: "down", sourceCursor: 7, sourceOffset: 5, targetCursor: 8, targetOffset: 8, targetCount: 12, want: 10},
		{name: "up arrow into scrolled row", key: "up", from: 1, sourceCursor: 6, sourceOffset: 5, targetCursor: 11, targetOffset: 8, targetCount: 12, want: 9},
		{name: "short row selects closest poster", key: "j", sourceCursor: 3, targetCount: 2, want: 1},
		{name: "last partial row selects closest poster", key: "down", sourceCursor: 3, targetCursor: 11, targetOffset: 10, targetCount: 12, want: 11},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ui.GetLayout().Update(ui.SidebarTotalWidth+ui.MainOverhead+4*ui.PosterStepWidth, 50, false)
			v := NewHomeView(ui.PlexctlTheme)
			v.loading = false
			for row := range 2 {
				list := poster.NewPosterList(row, fmt.Sprintf("Row %d", row), nil, ui.PlexctlTheme)
				count := 12
				if row != tt.from {
					count = tt.targetCount
				}
				for i := range count {
					list.Items = append(list.Items, &poster.PosterItem{Metadata: components.Metadata{
						Title: fmt.Sprintf("Row %d Movie %d", row, i),
					}})
				}
				v.lists = append(v.lists, list)
			}
			v.Update(tea.WindowSizeMsg{})
			source, target := v.lists[tt.from], v.lists[1-tt.from]
			source.Cursor, source.Offset, source.Active = tt.sourceCursor, tt.sourceOffset, true
			target.Cursor, target.Offset = tt.targetCursor, tt.targetOffset
			v.activeList = tt.from
			v.updateViewportContent()
			v.Update(ui.KeyPress(tt.key))
			wantTitle := fmt.Sprintf("Row %d Movie %d", 1-tt.from, tt.want)
			got := v.GetSelectedMetadata()
			if got == nil {
				t.Fatal("vertical movement lost the selection")
			}
			if got.Title != wantTitle {
				t.Fatalf("%s selected %q; want %q", tt.key, got.Title, wantTitle)
			}
			if target.Offset != tt.targetOffset {
				t.Fatal("vertical movement scrolled the destination row away from its visible posters")
			}
			if source.Active || !target.Active {
				t.Fatal("selection highlight did not move to the destination row")
			}
		})
	}
}

func TestHomeVerticalNavigationStopsAtEdges(t *testing.T) {
	v := NewHomeView(ui.PlexctlTheme)
	for _, key := range []string{"j", "k", "down", "up"} {
		v.Update(ui.KeyPress(key))
		if v.GetSelectedMetadata() != nil {
			t.Fatal("empty Home view gained a selection")
		}
	}
	list := poster.NewPosterList(0, "Movies", nil, ui.PlexctlTheme)
	list.Items = []*poster.PosterItem{{Metadata: components.Metadata{Title: "Only movie"}}}
	list.Active = true
	v.lists = []*poster.PosterList{list}
	for _, key := range []string{"j", "k", "down", "up"} {
		v.Update(ui.KeyPress(key))
		if v.GetSelectedMetadata().Title != "Only movie" || !list.Active {
			t.Fatal("vertical movement changed selection at the edge")
		}
	}
}
