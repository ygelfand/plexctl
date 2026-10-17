package detail

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestMusicNavigationAndMouse(t *testing.T) {
	ui.GetLayout().Update(100, 40, false)
	artist := NewMusicDetailView("artist", ui.PlexctlTheme)
	artist.Update(detailDataMsg{
		metadata: &components.Metadata{Type: "artist", Title: "Artist"},
		children: []components.Metadata{{RatingKey: ui.Ptr("album"), Type: "album", Title: "The album"}},
	})
	artist.Update(ui.KeyPress("enter"))
	album := artist.selected
	if album == nil || album.RatingKey != "album" {
		t.Fatal("enter did not open the selected album")
	}
	var tracks []components.Metadata
	for i := range 20 {
		tracks = append(tracks, components.Metadata{RatingKey: ui.Ptr(fmt.Sprint(i)), Type: "track", Title: fmt.Sprintf("Song %02d", i), Index: ui.Ptr(i + 1)})
	}
	artist.Update(detailDataMsg{metadata: &components.Metadata{Type: "album", Title: "The album"}, children: tracks})
	album.View() // Establish the list's actual viewport size.
	for range 10 {
		album.Update(tea.MouseMsg{X: album.listRect.X + 3, Y: album.listRect.Y + 3, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	view := ansi.Strip(album.View())
	clicked := false
	for y, line := range strings.Split(view, "\n") {
		if x := strings.Index(line, "Song "); x >= 0 {
			title := line[x : x+len("Song 00")]
			album.Update(tea.MouseMsg{X: lipgloss.Width(line[:x]) + 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			selected := album.items.SelectedItem().(musicItem)
			if album.selected == nil || selected.metadata.Title != title || album.selected.RatingKey != *selected.metadata.RatingKey {
				t.Fatalf("clicked %q but opened %+v", title, album.selected)
			}
			clicked = true
			break
		}
	}
	if !clicked {
		t.Fatal("no tracks rendered")
	}
	manager := DetailManager{View: artist}
	manager.Update(BackMsg{})
	if artist.selected != album || album.selected != nil {
		t.Fatal("back from track skipped its album")
	}
	if got := artist.GetSelectedMetadata(); got == nil || got.Type != "track" {
		t.Fatal("album did not expose its selected track for playback")
	}
	manager.Update(BackMsg{})
	if manager.View != artist || artist.selected != nil {
		t.Fatal("back from album skipped its artist")
	}
	manager.Update(BackMsg{})
	if manager.Active() {
		t.Fatal("back from artist did not return to the library")
	}
}

func TestArtistShowsAlbumArtworkAndSelectsCover(t *testing.T) {
	ui.GetLayout().Update(100, 40, false)
	artist := NewMusicDetailView("artist", ui.PlexctlTheme)
	artist.Update(detailDataMsg{
		metadata: &components.Metadata{Type: "artist", Title: "Artist"},
		children: []components.Metadata{
			{RatingKey: ui.Ptr("a1"), Type: "album", Title: "First album", Year: ui.Ptr(2024)},
			{RatingKey: ui.Ptr("a2"), Type: "album", Title: "Second album", Year: ui.Ptr(2026)},
		},
	})
	artist.Update(poster.PosterLoadedMsg{Index: 0, RatingKey: "a1", View: "FIRST COVER"})
	artist.Update(poster.PosterLoadedMsg{Index: 1, RatingKey: "a2", View: "SECOND COVER"})
	view := ansi.Strip(artist.View())
	if !strings.Contains(view, "SECOND COVER") || !strings.Contains(view, "2026") {
		t.Fatal("artist page did not show album artwork and year")
	}
	_, cmd := artist.Update(tea.MouseMsg{
		X: artist.albumsRect.X + ui.PosterStepWidth + 2, Y: artist.albumsRect.Y + 3,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	if cmd == nil {
		t.Fatal("album cover click did not open an album")
	}
	artist.Update(cmd())
	if artist.selected == nil || artist.selected.RatingKey != "a2" {
		t.Fatal("album cover click selected the wrong album")
	}
	artist.Update(BackMsg{})
	if selected := artist.GetSelectedMetadata(); selected == nil || ui.PtrToString(selected.RatingKey) != "a2" {
		t.Fatal("returning from an album lost the gallery selection")
	}
}

func TestAlbumArtworkBesideClickableTracks(t *testing.T) {
	for _, size := range [][2]int{{140, 45}, {83, 49}, {80, 24}} {
		ui.GetLayout().Update(size[0], size[1], false)
		album := NewMusicDetailView("album", ui.PlexctlTheme)
		album.Update(detailDataMsg{metadata: &components.Metadata{Type: "album", Title: "My album"},
			children: []components.Metadata{{RatingKey: ui.Ptr("song"), Type: "track", Title: "Zephyr"}}})
		album.Update(PosterDataMsg{ratingKey: "album", width: detailPosterWidth(album.Metadata), view: "COVER"})
		view := ansi.Strip(album.View())
		if !strings.Contains(view, "COVER") || !strings.Contains(view, "Zephyr") {
			t.Fatalf("%v: missing artwork or tracks:\n%s", size, view)
		}
		for y, line := range strings.Split(view, "\n") {
			if before, _, ok := strings.Cut(line, "Zephyr"); ok {
				album.Update(tea.MouseMsg{X: lipgloss.Width(before), Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			}
		}
		if album.selected == nil || album.selected.RatingKey != "song" {
			t.Fatalf("%v: cover shifted the track's mouse target (%+v):\n%s", size, album.listRect, view)
		}
		album.Update(PosterDataMsg{ratingKey: "album", width: detailPosterWidth(album.Metadata), view: "NEW COVER"})
		if album.Poster != "NEW COVER" || album.selected.Poster != "" {
			t.Fatal("late parent artwork was applied to the selected track")
		}
	}
}

func TestAlbumLettersUseTitlesAndPreserveTrackOrder(t *testing.T) {
	ui.GetLayout().Update(100, 40, false)
	artist := NewMusicDetailView("artist", ui.PlexctlTheme)
	artist.Update(detailDataMsg{metadata: &components.Metadata{Type: "artist", Title: "Artist"},
		children: []components.Metadata{
			{RatingKey: ui.Ptr("z"), Type: "album", Title: "Zebra"},
			{RatingKey: ui.Ptr("a"), Type: "album", Title: "Amber"},
		}})
	artist.JumpToLetter("Z")
	if artist.GetSelectedMetadata().Title != "Zebra" || artist.albums.Cursor != 1 {
		t.Fatal("artist albums were not sorted or jumped to by title")
	}
	artist.Update(ui.KeyPress("enter"))
	artist.Update(detailDataMsg{metadata: &components.Metadata{Type: "album", Title: "Zebra"},
		children: []components.Metadata{
			{RatingKey: ui.Ptr("song1"), Type: "track", Title: "Zulu", Index: ui.Ptr(1)},
			{RatingKey: ui.Ptr("song2"), Type: "track", Title: "Amber", Index: ui.Ptr(2)},
		}})
	artist.JumpToLetter("A")
	if artist.GetSelectedMetadata().Title != "Amber" || artist.selected.items.Index() != 1 {
		t.Fatal("track jump did not use the song title independently of track number")
	}
}

func TestCompactAlbumFitsArtworkTracksAlphabetAndFooter(t *testing.T) {
	ui.GetLayout().SetPlayerHeight(8)
	ui.GetLayout().Update(80, 24, true)
	defer ui.GetLayout().Update(80, 24, false)
	album := NewMusicDetailView("album", ui.PlexctlTheme)
	var tracks []components.Metadata
	for i := range 200 {
		tracks = append(tracks, components.Metadata{Type: "track", Title: fmt.Sprintf("Track %d", i)})
	}
	album.Update(detailDataMsg{metadata: &components.Metadata{Type: "album", Title: "Album"}, children: tracks})
	view := album.View()
	if lipgloss.Height(view) > ui.GetLayout().ContentHeight() || lipgloss.Width(view) > ui.GetLayout().InnerWidth() {
		t.Fatalf("compact album overflows its viewport: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	if !strings.Contains(view, "[esc] Back") || album.rail.Width == 0 {
		t.Fatal("compact album lost its footer or mouse alphabet")
	}
}

func TestMusicAlphabetFocusFollowsAlbumAndTrackLists(t *testing.T) {
	ui.GetLayout().Update(100, 40, false)
	artist := NewMusicDetailView("artist", ui.PlexctlTheme)
	if artist.AlphabetRail() != nil {
		t.Fatal("loading artist exposed an invisible rail")
	}
	artist.Update(detailDataMsg{metadata: &components.Metadata{Type: "artist", Title: "Artist"},
		children: []components.Metadata{
			{RatingKey: ui.Ptr("a"), Type: "album", Title: "Amber"},
			{RatingKey: ui.Ptr("z"), Type: "album", Title: "Zebra"},
		}})
	artist.View()
	albumsRail := artist.AlphabetRail()
	albumsRail.Focus("Z")
	if !strings.Contains(artist.View(), "[Z]") || artist.GetSelectedMetadata().Title != "Amber" {
		t.Fatal("album rail rendering lost focus or changed the selected album")
	}
	artist.JumpToLetter(albumsRail.SelectedLetter())
	albumsRail.Focused = false
	artist.Update(ui.KeyPress("enter"))
	if artist.AlphabetRail() != nil {
		t.Fatal("loading album exposed the parent artist's rail")
	}
	artist.Update(detailDataMsg{metadata: &components.Metadata{Type: "album", Title: "Zebra"},
		children: []components.Metadata{
			{RatingKey: ui.Ptr("song1"), Type: "track", Title: "Zulu", Index: ui.Ptr(1)},
			{RatingKey: ui.Ptr("song2"), Type: "track", Title: "Amber", Index: ui.Ptr(2)},
		}})
	artist.View()
	tracksRail := artist.AlphabetRail()
	if tracksRail == nil || tracksRail == albumsRail {
		t.Fatal("album did not expose its own track-list rail")
	}
	tracksRail.Focus("A")
	for _, size := range [][2]int{{80, 24}, {100, 40}} {
		ui.GetLayout().Update(size[0], size[1], false)
		if !strings.Contains(artist.View(), "[A]") || artist.GetSelectedMetadata().Title != "Zulu" {
			t.Fatal("track-list resize lost focus or changed the selected track")
		}
	}
	artist.JumpToLetter(tracksRail.SelectedLetter())
	if artist.GetSelectedMetadata().Title != "Amber" {
		t.Fatal("rail did not jump by track title")
	}
	tracksRail.Focused = false
	artist.Update(ui.KeyPress("enter"))
	artist.Update(detailDataMsg{metadata: &components.Metadata{Type: "track", Title: "Amber"}})
	if artist.AlphabetRail() != nil {
		t.Fatal("individual track exposed an invisible rail")
	}
}
