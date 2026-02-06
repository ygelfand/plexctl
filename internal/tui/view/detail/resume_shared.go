package detail

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/tui/player"
)

func (v *MovieDetailView) checkResume(tctMode bool) tea.Cmd {
	if v.Metadata == nil || v.Metadata.RatingKey == nil {
		return nil
	}
	return player.FetchAndPlay(*v.Metadata.RatingKey, tctMode)
}

func (v *EpisodeDetailView) checkResume(tctMode bool) tea.Cmd {
	if v.Metadata == nil || v.Metadata.RatingKey == nil {
		return nil
	}
	return player.FetchAndPlay(*v.Metadata.RatingKey, tctMode)
}
