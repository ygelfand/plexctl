package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/ui"
)

// ContentWalker is a function that fetches a single page of results
type ContentWalker func(ctx context.Context, start, size int) ([]components.Metadata, int64, error)

// WalkContent handles either fetching a single page or walking all pages based on the provided options
func WalkContent(ctx context.Context, all bool, page, count int, walker ContentWalker) ([]components.Metadata, error) {
	var allMetadata []components.Metadata
	start := 0
	size := count

	if all {
		size = 100 // doesn't seem to make much of a difference how many per page once it gets here
	} else {
		if page < 1 || count < 1 {
			return nil, fmt.Errorf("page and count must be positive")
		}
		if page-1 > math.MaxInt/count {
			return nil, fmt.Errorf("pagination offset is out of range")
		}
		start = (page - 1) * count
	}

	previousPage := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		metadata, totalSize, err := walker(ctx, start, size)
		if err != nil {
			return nil, err
		}

		if !all {
			return metadata, nil
		}
		if len(metadata) == 0 {
			if totalSize > int64(start) {
				return nil, fmt.Errorf("server returned an incomplete content listing")
			}
			break
		}
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return nil, err
		}
		if string(encoded) == previousPage {
			return nil, fmt.Errorf("server did not advance content pagination")
		}
		previousPage = string(encoded)
		allMetadata = append(allMetadata, metadata...)

		start += len(metadata)
		if totalSize > 0 && int64(start) >= totalSize {
			break
		}
	}

	return allMetadata, nil
}

// LibraryWalker returns a ContentWalker for a specific library section
func LibraryWalker(client *Client, sectionID string) ContentWalker {
	return func(ctx context.Context, start, size int) ([]components.Metadata, int64, error) {
		req := operations.ListContentRequest{
			SectionID:           sectionID,
			XPlexContainerStart: ui.Ptr(start),
			XPlexContainerSize:  ui.Ptr(size),
		}

		res, err := client.SDK.Content.ListContent(ctx, req)
		if err != nil {
			return nil, 0, err
		}

		if res.MediaContainerWithMetadata == nil || res.MediaContainerWithMetadata.MediaContainer == nil {
			return nil, 0, nil
		}

		mc := res.MediaContainerWithMetadata.MediaContainer
		total := int64(0)
		if mc.TotalSize != nil {
			total = *mc.TotalSize
		}

		return mc.Metadata, total, nil
	}
}
