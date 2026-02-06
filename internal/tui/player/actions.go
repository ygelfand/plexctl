package player

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

func PlayMedia(metadata *components.Metadata, noReport bool, tctMode bool, startOffset int64) tea.Cmd {
	if metadata == nil || len(metadata.Media) == 0 || len(metadata.Media[0].Part) == 0 {
		return func() tea.Msg { return fmt.Errorf("no playable media found") }
	}

	cfg := config.Get()
	_, serverCfg, ok := cfg.GetActiveServer()
	if !ok {
		return func() tea.Msg { return fmt.Errorf("no active server") }
	}

	part := metadata.Media[0].Part[0]
	separator := "?"
	if strings.Contains(part.Key, "?") {
		separator = "&"
	}
	playURL := fmt.Sprintf("%s%s%sX-Plex-Token=%s", serverCfg.URL, part.Key, separator, cfg.Token)
	slog.Debug("Playing media", "url", playURL, "tct", tctMode, "offset", startOffset)
	rk := ""
	if metadata.RatingKey != nil {
		rk = *metadata.RatingKey
	}

	title := metadata.Title
	if noReport {
		title = "Trailer: " + title
	}

	return GetPlayerManager().Play(playURL, title, rk, noReport, tctMode, startOffset)
}

// FetchAndPlay handles the full playback logic: fetch full metadata, check for resume, then play
func FetchAndPlay(ratingKey string, tctMode bool) tea.Cmd {
	return func() tea.Msg {
		meta, err := plex.GetMetadata(context.Background(), ratingKey, true)
		if err != nil {
			return err
		}

		if meta.ViewOffset != nil && *meta.ViewOffset > 0 {
			return ui.ResumeChoiceMsg{
				Metadata: meta,
				TctMode:  tctMode,
			}
		}

		cmd := PlayMedia(meta, false, tctMode, 0)
		if cmd != nil {
			return cmd()
		}
		return nil
	}
}
