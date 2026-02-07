package presenters

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/ui"
)

type HistoryItem struct {
	Date    string
	User    string
	Title   string
	Type    string
	Device  string
	Library string
}

func (i HistoryItem) ToRow() []string {
	return []string{
		i.Date,
		i.User,
		i.Title,
		i.Type,
		i.Device,
		i.Library,
	}
}

type HistoryPresenter struct {
	Items   []HistoryItem
	RawData []operations.ListPlaybackHistoryMetadata
}

func (p *HistoryPresenter) Title() string {
	return "Playback History"
}

func (p *HistoryPresenter) Headers() []string {
	return []string{"DATE", "USER", "TITLE", "TYPE", "DEVICE", "LIBRARY"}
}

func (p *HistoryPresenter) Rows() [][]string {
	var rows [][]string
	for _, i := range p.Items {
		rows = append(rows, i.ToRow())
	}
	return rows
}

func (p *HistoryPresenter) Raw() interface{} {
	return p.RawData
}

func (p *HistoryPresenter) SortableColumns() []string {
	return []string{"date", "user", "title", "type", "device", "library"}
}

func (p *HistoryPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "date":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Date > p.Items[j].Date })
	case "user":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].User < p.Items[j].User })
	case "title":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Title < p.Items[j].Title })
	case "type":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Type < p.Items[j].Type })
	case "device":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Device < p.Items[j].Device })
	case "library":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Library < p.Items[j].Library })
	default:
		return false
	}
	return true
}

func (p *HistoryPresenter) DefaultSort() string {
	return "date"
}

// MapHistoryMetadata maps history metadata to presenter items with enhanced titles
func MapHistoryMetadata(m []operations.ListPlaybackHistoryMetadata, userMap map[int64]string, libMap map[string]string, deviceMap map[string]string) []HistoryItem {
	res := make([]HistoryItem, len(m))
	for i, meta := range m {
		viewedAt := ""
		if meta.ViewedAt != nil {
			viewedAt = time.Unix(*meta.ViewedAt, 0).Format("2006-01-02 15:04")
		}

		user := "Unknown"
		if meta.AccountID != nil {
			if u, ok := userMap[*meta.AccountID]; ok {
				user = u
			} else {
				user = fmt.Sprintf("%d", *meta.AccountID)
			}
		}

		title := ui.PtrToString(meta.Title)
		mType := ui.PtrToString(meta.Type)

		// Enhance title for episodes using the newly added SDK fields
		if mType == "episode" {
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

		device := "Unknown"
		if meta.DeviceID != nil {
			dKey := fmt.Sprintf("%d", *meta.DeviceID)
			if d, ok := deviceMap[dKey]; ok {
				device = d
			} else {
				device = dKey
			}
		}

		library := "Unknown"
		if meta.LibrarySectionID != nil {
			if l, ok := libMap[*meta.LibrarySectionID]; ok {
				library = l
			} else {
				library = *meta.LibrarySectionID
			}
		}

		res[i] = HistoryItem{
			Date:    viewedAt,
			User:    user,
			Title:   title,
			Type:    mType,
			Device:  device,
			Library: library,
		}
	}
	return res
}
