package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
)

// SectionIDOf returns the section carried by Plex's extensible metadata.
func SectionIDOf(meta *components.Metadata) string {
	if meta == nil {
		return ""
	}
	switch value := meta.AdditionalProperties["librarySectionID"].(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case json.Number:
		return value.String()
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	default:
		return ""
	}
}

// GetArtistAlbums includes soundtracks and compilations where the artist is a
// track artist, even when the album belongs to a different album artist.
func GetArtistAlbums(ctx context.Context, sectionID, artistKey string) ([]components.Metadata, error) {
	return artistListing(ctx, sectionID, artistKey, "9")
}

func GetArtistTracks(ctx context.Context, sectionID, artistKey string) ([]components.Metadata, error) {
	return artistListing(ctx, sectionID, artistKey, "10")
}

func artistListing(ctx context.Context, sectionID, artistKey, mediaType string) ([]components.Metadata, error) {
	query := url.Values{"artist.id": {artistKey}, "type": {mediaType}}
	return metadataListing(ctx, "/library/sections/"+url.PathEscape(sectionID)+"/all", query)
}

// GetMusicChildren uses the same artist query for browsing and playback.
func GetMusicChildren(ctx context.Context, meta *components.Metadata) ([]components.Metadata, error) {
	if meta == nil || meta.RatingKey == nil {
		return nil, fmt.Errorf("music item has no rating key")
	}
	if meta.Type == "artist" {
		if section := SectionIDOf(meta); section != "" {
			return GetArtistAlbums(ctx, section, *meta.RatingKey)
		}
	}
	return GetChildren(ctx, *meta.RatingKey)
}

// CollectTracks retains Plex's order and resolves every playable track. A
// visited set handles duplicate/cyclic containers without dropping distinct
// recordings of the same song. Request failures abort rather than silently
// playing an incomplete album.
func CollectTracks(ctx context.Context, meta *components.Metadata) ([]components.Metadata, error) {
	tracks := []components.Metadata{}
	visited := make(map[string]bool)
	var walk func(*components.Metadata) error
	walk = func(item *components.Metadata) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item == nil {
			return nil
		}
		key := ""
		if item.RatingKey != nil {
			key = *item.RatingKey
		}
		if key != "" {
			if visited[key] {
				return nil
			}
			visited[key] = true
		}
		if item.Type == "track" {
			if PlayablePart(item) == nil && key != "" {
				full, err := GetMetadata(ctx, key, false)
				if err != nil {
					return err
				}
				item = full
			}
			if PlayablePart(item) != nil {
				tracks = append(tracks, *item)
			}
			return nil
		}
		if key == "" || (item.Type != "artist" && item.Type != "album") {
			return nil
		}
		var children []components.Metadata
		var err error
		if section := SectionIDOf(item); item.Type == "artist" && section != "" {
			children, err = GetArtistTracks(ctx, section, key)
		} else {
			children, err = GetChildren(ctx, key)
		}
		if err != nil {
			return err
		}
		for i := range children {
			if err := walk(&children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(meta); err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		return nil, fmt.Errorf("no playable tracks found")
	}
	return tracks, nil
}

// metadataListing follows Plex pagination and namespaces the cache by server
// and credentials. Tokens travel in headers, keeping HTTP errors safe to show.
func metadataListing(ctx context.Context, path string, query url.Values) ([]components.Metadata, error) {
	server := config.ServerFromContext(ctx)
	if server.URL == "" {
		return nil, fmt.Errorf("no active server")
	}
	cm, err := cache.Get(config.Get().CacheDir)
	if err != nil {
		return nil, err
	}
	cacheKey := server.CacheNamespace() + "/listing/" + path + "?" + query.Encode()
	items := []components.Metadata{}
	if err := cm.Get(cacheKey, &items); err == nil {
		return items, nil
	}
	const pageSize = 200
	previousPage := ""
	for start := 0; ; {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		u, err := mediaURL(server, path+"?"+query.Encode())
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Plex-Token", server.Token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Container-Start", strconv.Itoa(start))
		req.Header.Set("X-Plex-Container-Size", strconv.Itoa(pageSize))
		resp, err := mediaHTTPClient().Do(req)
		if err != nil {
			return nil, err
		}
		var body components.MediaContainerWithMetadata
		decodeErr := json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch music listing: %s", resp.Status)
		}
		if decodeErr != nil {
			return nil, fmt.Errorf("decode music listing: %w", decodeErr)
		}
		if body.MediaContainer == nil {
			return nil, fmt.Errorf("no media container in listing")
		}
		page := body.MediaContainer
		if len(page.Metadata) == 0 {
			if page.TotalSize != nil && int64(start) < *page.TotalSize {
				return nil, fmt.Errorf("server returned an incomplete music listing")
			}
			break
		}
		// Detect servers that ignore the requested offset, avoiding an
		// unbounded queue while still accepting an unpaginated response.
		encoded, err := json.Marshal(page.Metadata)
		if err != nil {
			return nil, err
		}
		if string(encoded) == previousPage {
			return nil, fmt.Errorf("server did not advance music listing pagination")
		}
		previousPage = string(encoded)
		items = append(items, page.Metadata...)
		start += len(page.Metadata)
		if page.TotalSize != nil {
			if int64(start) >= *page.TotalSize {
				break
			}
		} else if len(page.Metadata) < pageSize {
			break
		}
	}
	_ = cm.Set(cacheKey, items, MediaCacheTTL)
	return items, nil
}
