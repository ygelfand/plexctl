package plex

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestWalkContentValidatesOffsets(t *testing.T) {
	for _, tt := range []struct {
		name        string
		page, count int
	}{
		{"zero page", 0, 10},
		{"negative page", -1, 10},
		{"zero count", 1, 0},
		{"negative count", 1, -10},
		{"overflow", math.MaxInt, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := WalkContent(t.Context(), false, tt.page, tt.count, func(context.Context, int, int) ([]components.Metadata, int64, error) {
				t.Fatal("invalid pagination reached the server")
				return nil, 0, nil
			})
			if err == nil {
				t.Fatal("invalid pagination succeeded")
			}
		})
	}
}

func TestWalkContentRejectsBrokenPagination(t *testing.T) {
	for _, tt := range []struct {
		name     string
		repeated bool
	}{
		{"repeated page", true},
		{"empty incomplete page", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			_, err := WalkContent(t.Context(), true, 1, 10, func(_ context.Context, start, size int) ([]components.Metadata, int64, error) {
				calls++
				if calls > 2 {
					t.Fatal("walker continued after server failed to advance")
				}
				if tt.repeated || start == 0 {
					return []components.Metadata{{RatingKey: ptr("1"), Title: "first"}}, 3, nil
				}
				return nil, 3, nil
			})
			if err == nil {
				t.Fatal("broken pagination returned an apparently complete listing")
			}
		})
	}
}

func TestWalkContentRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	_, err := WalkContent(ctx, true, 1, 10, func(context.Context, int, int) ([]components.Metadata, int64, error) {
		calls++
		cancel()
		return []components.Metadata{{RatingKey: ptr("1")}}, 2, nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("canceled pagination: error=%v, calls=%d", err, calls)
	}
}

func TestWalkContentCollectsPages(t *testing.T) {
	items, err := WalkContent(t.Context(), true, 0, 0, func(_ context.Context, start, size int) ([]components.Metadata, int64, error) {
		if size != 100 {
			t.Errorf("unexpected page size %d", size)
		}
		switch start {
		case 0:
			return []components.Metadata{{Title: "first"}, {Title: "second"}}, 3, nil
		case 2:
			return []components.Metadata{{Title: "third"}}, 3, nil
		default:
			t.Fatalf("unexpected offset %d", start)
			return nil, 0, nil
		}
	})
	if err != nil || len(items) != 3 || items[2].Title != "third" {
		t.Fatalf("pagination: %#v, %v", items, err)
	}
}
