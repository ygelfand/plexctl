package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

// Live searches the server's complete library, including music that has never
// been paged into the TUI or indexed locally.
func Live(ctx context.Context, query string) ([]IndexEntry, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	client, err := plex.NewClientForContext(ctx)
	if err != nil {
		return nil, err
	}
	response, err := client.SDK.Search.SearchHubs(ctx, operations.SearchHubsRequest{
		Query: query, Limit: ui.Ptr(int64(50)),
	})
	if err != nil {
		return nil, err
	}
	if response.Object == nil || response.Object.MediaContainer == nil {
		return nil, fmt.Errorf("search returned no media container")
	}
	entries := []IndexEntry{}
	seen := map[string]bool{}
	for _, hub := range response.Object.MediaContainer.Hub {
		for _, meta := range hub.Metadata {
			switch meta.Type {
			case "artist", "album", "track", "movie", "show", "season", "episode":
			default:
				continue
			}
			key := ui.PtrToString(meta.RatingKey)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			sectionID, library := "", ""
			if value, ok := meta.AdditionalProperties["librarySectionID"]; ok {
				sectionID = fmt.Sprint(value)
			}
			if value, ok := meta.AdditionalProperties["librarySectionTitle"].(string); ok {
				library = value
			}
			entries = append(entries, entryFromMetadata(meta, sectionID, library))
		}
	}
	return entries, nil
}
