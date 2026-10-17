package view

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/ui"
)

func artistPage(start, count int) []components.Metadata {
	items := make([]components.Metadata, count)
	for i := range items {
		items[i] = components.Metadata{RatingKey: ui.Ptr(fmt.Sprint(start + i)), Title: "Artist", Type: "artist"}
	}
	return items
}

func TestLetterJumpLoadsMissingPagesAndPreservesSelectionOnFailure(t *testing.T) {
	ui.GetLayout().Update(100, 35, false)
	v := NewMediaView("music", "Music", ui.PlexctlTheme)
	v.viewMode = ViewModePoster
	v.Update(tea.WindowSizeMsg{Width: 100, Height: 35})
	v.Init()
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Metadata: artistPage(0, 100), Total: 237})
	v.JumpToLetter("Z")
	if !v.isLoading || v.jumpLetter != "Z" {
		t.Fatal("jump did not request missing metadata")
	}
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Start: 100, Err: fmt.Errorf("offline")})
	if v.posterGrid.Cursor != 0 || !strings.Contains(v.View(), "Retry") {
		t.Fatal("failed jump lost the library or its selection")
	}
	v.Update(ui.KeyPress("r"))
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Start: 100, Metadata: artistPage(100, 100), Total: 237})
	if !v.isLoading {
		t.Fatal("jump stopped before reaching the requested letter")
	}
	last := artistPage(200, 37)
	last[36].Title = "Zephyr"
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Start: 200, Metadata: last, Total: 237})
	if v.posterGrid.Cursor != 236 || v.table.Cursor() != 236 || v.jumpLetter != "" || v.hasMore {
		t.Fatal("jump did not select the first matching artist on the last page")
	}
	if !strings.Contains(v.View(), "Zephyr") {
		t.Fatal("jump selected an artist without scrolling it into view")
	}
	v.JumpToLetter("B")
	if v.posterGrid.Cursor != 236 || !strings.Contains(v.View(), "No titles under B") {
		t.Fatal("missing letter moved the selection or gave no feedback")
	}
	v.Update(ui.KeyPress("home"))
	if v.posterGrid.Cursor != 0 {
		t.Fatal("Home did not restore the first item")
	}
}

func TestLetterJumpCanBeChangedOrCancelledWhileLoading(t *testing.T) {
	v := NewMediaView("music", "Music", ui.PlexctlTheme)
	v.Init()
	v.JumpToLetter("Z")
	v.JumpToLetter("B")
	page := artistPage(0, 100)
	page[50].Title = "B artist"
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Metadata: page, Total: 200})
	if v.table.Cursor() != 50 || v.isLoading {
		t.Fatal("an earlier jump overrode the latest destination")
	}
	v.JumpToLetter("Z")
	v.Update(ui.KeyPress("esc"))
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Start: 100, Metadata: artistPage(100, 100), Total: 300})
	if v.isLoading || v.table.Cursor() != 50 || v.jumpLetter != "" {
		t.Fatal("cancelled jump kept paging or changed selection")
	}
}

func TestLibraryEndLoadsFinalPage(t *testing.T) {
	v := NewMediaView("music", "Music", ui.PlexctlTheme)
	v.Init()
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Metadata: artistPage(0, 100), Total: 110})
	v.Update(ui.KeyPress("end"))
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Start: 100, Metadata: artistPage(100, 10), Total: 110})
	if v.table.Cursor() != 109 || v.jumpEnd || v.isLoading {
		t.Fatal("End did not reach the final library item")
	}
}

func TestMousePaginationFollowsViewport(t *testing.T) {
	ui.GetLayout().Update(100, 35, false)
	v := NewMediaView("music", "Music", ui.PlexctlTheme)
	v.viewMode = ViewModePoster
	v.Update(tea.WindowSizeMsg{Width: 100, Height: 35})
	v.Init()
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Metadata: artistPage(0, 100), Total: 237})
	for _, start := range []int{100, 200} {
		v.View()
		for i := 0; !v.isLoading && i < 300; i++ {
			v.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		}
		if !v.isLoading || v.posterGrid.Cursor != 0 {
			t.Fatalf("scrolling did not load page %d independently of selection", start)
		}
		requestID := v.requestID
		v.Update(ui.KeyPress("down"))
		if v.requestID != requestID {
			t.Fatal("issued a duplicate page request while loading")
		}
		v.posterGrid.Cursor = 0
		v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: requestID, Start: start,
			Metadata: artistPage(start, min(100, 237-start)), Total: 237})
	}
	if v.loadedItems != 237 || v.hasMore {
		t.Fatalf("last page: loaded %d, hasMore %v", v.loadedItems, v.hasMore)
	}
}

func TestPaginationIgnoresStaleResponsesAndAllowsRetry(t *testing.T) {
	v := NewMediaView("music", "Music", ui.PlexctlTheme)
	v.Init()
	oldID := v.requestID
	v.Refresh()
	for _, response := range []ui.MediaPageMsg{
		{SectionID: "other", RequestID: v.requestID, Metadata: artistPage(0, 10)},
		{SectionID: "music", RequestID: oldID, Metadata: artistPage(0, 10)},
	} {
		v.Update(response)
	}
	if v.loadedItems != 0 || !v.isLoading {
		t.Fatal("a stale page changed the refreshed library")
	}
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Err: fmt.Errorf("offline")})
	if v.isLoading || v.err == nil {
		t.Fatal("page failure left the library loading")
	}
	v.Update(ui.KeyPress("r"))
	if !v.isLoading || v.err != nil {
		t.Fatal("retry did not restart the failed page")
	}
}

func TestUnknownTotalContinuesUntilEmptyPage(t *testing.T) {
	v := NewMediaView("music", "Music", ui.PlexctlTheme)
	v.Init()
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Metadata: artistPage(0, 7), Total: -1})
	if !v.hasMore || !v.isLoading {
		t.Fatal("short page without total prematurely ended pagination")
	}
	v.Update(ui.MediaPageMsg{SectionID: "music", RequestID: v.requestID, Start: 7, Total: -1})
	if v.hasMore || v.isLoading || v.loadedItems != 7 {
		t.Fatal("empty page did not finish pagination")
	}
}

func TestLibraryAlphabetFocusSurvivesResize(t *testing.T) {
	for _, mediaType := range []string{"movie", "show", "artist"} {
		t.Run(mediaType, func(t *testing.T) {
			ui.GetLayout().Update(100, 40, false)
			v := NewMediaView("library", "Library", ui.PlexctlTheme)
			v.Init()
			v.Update(ui.MediaPageMsg{SectionID: "library", RequestID: v.requestID, Total: 2,
				Metadata: []components.Metadata{
					{RatingKey: ui.Ptr("a"), Type: mediaType, Title: "Amber"},
					{RatingKey: ui.Ptr("z"), Type: mediaType, Title: "Zebra"},
				}})
			v.View()
			rail := v.AlphabetRail()
			if rail == nil || rail.Width == 0 {
				t.Fatal("library did not expose a visible alphabet rail")
			}
			rail.Focus("Z")
			for _, mode := range []ViewMode{ViewModePoster, ViewModeList} {
				v.viewMode = mode
				ui.GetLayout().Update(80, 24, false)
				v.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
				if !strings.Contains(v.View(), "[Z]") || v.GetSelectedMetadata().Title != "Amber" {
					t.Fatal("rendering or resizing lost the rail cursor or changed the title")
				}
			}
			v.JumpToLetter(rail.SelectedLetter())
			if v.GetSelectedMetadata().Title != "Zebra" {
				t.Fatal("jump did not select the requested title")
			}
		})
	}
}
