package plex

import (
	"context"
	"fmt"

	"github.com/LukeHagar/plexgo/models/components"
)

// PlayablePart finds the first version with a streamable file. Container items
// such as artists and albums have children instead of Media/Part entries.
func PlayablePart(meta *components.Metadata) *components.Part {
	if meta != nil {
		for i := range meta.Media {
			for j := range meta.Media[i].Part {
				part := &meta.Media[i].Part[j]
				if part.Key != "" {
					return part
				}
			}
		}
	}
	return nil
}

// ResolvePlayable resolves an artist or album to its first playable track in
// Plex's library order. Individual tracks and videos retain their own identity.
func ResolvePlayable(ctx context.Context, meta *components.Metadata) (*components.Metadata, error) {
	visited := make(map[string]bool)
	var find func(*components.Metadata) (*components.Metadata, error)
	find = func(item *components.Metadata) (*components.Metadata, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if PlayablePart(item) != nil {
			return item, nil
		}
		if item == nil || item.RatingKey == nil || visited[*item.RatingKey] {
			return nil, nil
		}
		visited[*item.RatingKey] = true
		if item.Type != "artist" && item.Type != "album" {
			return nil, nil
		}
		children, err := GetChildren(ctx, *item.RatingKey)
		if err != nil {
			return nil, err
		}
		for i := range children {
			child := &children[i]
			if child.RatingKey == nil {
				continue
			}
			if child.Type == "track" {
				child, err = GetMetadata(ctx, *child.RatingKey, true)
				if err != nil {
					return nil, err
				}
			} else if child.Type != "album" {
				continue
			}
			playable, err := find(child)
			if err != nil || playable != nil {
				return playable, err
			}
		}
		return nil, nil
	}
	playable, err := find(meta)
	if err != nil || playable != nil {
		return playable, err
	}
	if meta != nil && (meta.Type == "artist" || meta.Type == "album") {
		return nil, fmt.Errorf("no playable tracks found in this %s", meta.Type)
	}
	return nil, fmt.Errorf("no playable media found")
}
