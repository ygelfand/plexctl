package presenters

import (
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/ygelfand/plexctl/internal/ui"
)

// GenericMetadata is a common interface for different Plex metadata types
type GenericMetadata struct {
	ID            string
	Title         string
	Type          string
	Year          string
	Duration      string
	Studio        string
	Rating        string
	ContentRating string
	Genre         string
	Watched       string
}

// ToRow returns the metadata as a slice of strings for table rendering.
// Order: WATCHED, TITLE, TYPE, YEAR, DURATION, RATING, CONTENT, GENRE
func (m GenericMetadata) ToRow() []string {
	return []string{
		m.Watched,
		m.Title,
		m.Type,
		m.Year,
		m.Duration,
		m.Rating,
		m.ContentRating,
		m.Genre,
	}
}

// GetWatchedStatus returns a string representation of the watched status
func GetWatchedStatus(viewCount *int, viewOffset *int) string {
	if viewCount != nil && *viewCount > 0 {
		return "✓"
	}
	if viewOffset != nil && *viewOffset > 0 {
		return "●"
	}
	return ""
}

// MapMetadata maps standard metadata to generic metadata
func MapMetadata(m []components.Metadata) []GenericMetadata {
	res := make([]GenericMetadata, len(m))
	for i, meta := range m {
		id := ""
		if meta.RatingKey != nil {
			id = *meta.RatingKey
		}
		year := ""
		if meta.Year != nil {
			year = fmt.Sprintf("%d", *meta.Year)
		}
		duration := ""
		if meta.Duration != nil {
			duration = ui.FormatDuration(*meta.Duration)
		}
		studio := ""
		if meta.Studio != nil {
			studio = *meta.Studio
		}
		rating := ""
		if meta.Rating != nil {
			rating = fmt.Sprintf("%.1f", *meta.Rating)
		}
		contentRating := ""
		if meta.ContentRating != nil {
			contentRating = *meta.ContentRating
		}
		genre := ""
		if len(meta.Genre) > 0 {
			genre = meta.Genre[0].Tag
		}

		title := meta.Title
		if meta.Type == "episode" {
			parts := []string{}
			if meta.GrandparentTitle != nil {
				parts = append(parts, *meta.GrandparentTitle)
			}
			if meta.ParentIndex != nil && meta.Index != nil {
				parts = append(parts, fmt.Sprintf("S%02dE%02d", *meta.ParentIndex, *meta.Index))
			}
			if title != "" {
				parts = append(parts, title)
			}
			if len(parts) > 0 {
				title = strings.Join(parts, " / ")
			}
		}

		res[i] = GenericMetadata{
			ID:            id,
			Title:         title,
			Type:          meta.Type,
			Year:          year,
			Duration:      duration,
			Studio:        studio,
			Rating:        rating,
			ContentRating: contentRating,
			Genre:         genre,
			Watched:       GetWatchedStatus(meta.ViewCount, meta.ViewOffset),
		}
	}
	return res
}

// MapPlaylistMetadata maps playlist metadata to generic metadata
func MapPlaylistMetadata(m []components.MediaContainerWithPlaylistMetadataMetadata) []GenericMetadata {
	res := make([]GenericMetadata, len(m))
	for i, meta := range m {
		year := ""
		if meta.Year != nil {
			year = fmt.Sprintf("%d", *meta.Year)
		}
		duration := ""
		if meta.Duration != nil {
			duration = ui.FormatDuration(*meta.Duration)
		}
		studio := ""
		if meta.Studio != nil {
			studio = *meta.Studio
		}
		rating := ""
		if meta.Rating != nil {
			rating = fmt.Sprintf("%.1f", *meta.Rating)
		}
		contentRating := ""
		if meta.ContentRating != nil {
			contentRating = *meta.ContentRating
		}
		genre := ""
		if len(meta.Genre) > 0 {
			genre = meta.Genre[0].Tag
		}

		id := ""
		if meta.RatingKey != nil {
			id = *meta.RatingKey
		}

		title := meta.Title
		if meta.Type == "episode" {
			parts := []string{}
			if meta.GrandparentTitle != nil {
				parts = append(parts, *meta.GrandparentTitle)
			}
			if meta.ParentIndex != nil && meta.Index != nil {
				parts = append(parts, fmt.Sprintf("S%02dE%02d", *meta.ParentIndex, *meta.Index))
			}
			if title != "" {
				parts = append(parts, title)
			}
			if len(parts) > 0 {
				title = strings.Join(parts, " / ")
			}
		}

		res[i] = GenericMetadata{
			ID:            id,
			Title:         title,
			Type:          meta.Type,
			Year:          year,
			Duration:      duration,
			Studio:        studio,
			Rating:        rating,
			ContentRating: contentRating,
			Genre:         genre,
			Watched:       GetWatchedStatus(meta.ViewCount, meta.ViewOffset),
		}
	}
	return res
}
