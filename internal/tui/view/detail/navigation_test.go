package detail

import (
	"testing"

	"github.com/ygelfand/plexctl/internal/ui"
)

func TestBackFromEpisodePreservesSeasonAndShow(t *testing.T) {
	show := NewShowDetailView("show", ui.PlexctlTheme)
	season := NewSeasonDetailView("season", ui.PlexctlTheme)
	season.selectedEpisode = NewEpisodeDetailView("episode", ui.PlexctlTheme)
	show.selectedSeason = season
	manager := DetailManager{View: show}
	manager.Update(BackMsg{})
	if manager.View != show || show.selectedSeason != season || season.selectedEpisode != nil {
		t.Fatal("back from episode skipped its season")
	}
	manager.Update(BackMsg{})
	if manager.View != show || show.selectedSeason != nil {
		t.Fatal("back from season skipped its show")
	}
	manager.Update(BackMsg{})
	if manager.Active() {
		t.Fatal("back from show did not return to library")
	}
}
