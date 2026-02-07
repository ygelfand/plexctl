package presenters

import (
	"fmt"
	"sort"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
)

// LibraryListPresenter formats a list of libraries
type LibraryListPresenter struct {
	Directories []components.LibrarySection
}

func (p *LibraryListPresenter) Title() string {
	return "Plex Libraries"
}

func (p *LibraryListPresenter) Headers() []string {
	return []string{"ID", "TITLE", "TYPE", "AGENT", "LANGUAGE", "SCANNER", "LOCATION"}
}

func (p *LibraryListPresenter) Rows() [][]string {
	var rows [][]string
	for _, dir := range p.Directories {
		id := ""
		if dir.Key != nil {
			id = *dir.Key
		}
		title := ""
		if dir.Title != nil {
			title = *dir.Title
		}
		agent := ""
		if dir.Agent != nil {
			agent = *dir.Agent
		}
		language := dir.Language
		scanner := ""
		if dir.Scanner != nil {
			scanner = *dir.Scanner
		}

		location := ""
		if len(dir.Location) > 0 {
			var paths []string
			for _, loc := range dir.Location {
				if path, ok := loc.Path.(string); ok {
					paths = append(paths, path)
				}
			}
			location = strings.Join(paths, ", ")
		}

		rows = append(rows, []string{id, title, string(dir.Type), agent, language, scanner, location})
	}
	return rows
}

func (p *LibraryListPresenter) Raw() interface{} {
	return p.Directories
}

func (p *LibraryListPresenter) SortableColumns() []string {
	return []string{"id", "title", "type", "language"}
}

func (p *LibraryListPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Directories, func(i, j int) bool {
			// Basic string comparison for ID as it's a string pointer
			idI := ""
			if p.Directories[i].Key != nil {
				idI = *p.Directories[i].Key
			}
			idJ := ""
			if p.Directories[j].Key != nil {
				idJ = *p.Directories[j].Key
			}
			return idI < idJ
		})
	case "title":
		sort.Slice(p.Directories, func(i, j int) bool {
			tI := ""
			if p.Directories[i].Title != nil {
				tI = *p.Directories[i].Title
			}
			tJ := ""
			if p.Directories[j].Title != nil {
				tJ = *p.Directories[j].Title
			}
			return tI < tJ
		})
	case "type":
		sort.Slice(p.Directories, func(i, j int) bool {
			return string(p.Directories[i].Type) < string(p.Directories[j].Type)
		})
	case "language":
		sort.Slice(p.Directories, func(i, j int) bool {
			return p.Directories[i].Language < p.Directories[j].Language
		})
	default:
		return false
	}
	return true
}

func (p *LibraryListPresenter) DefaultSort() string {
	return "title"
}

// LibraryItemsPresenter formats items within a library
type LibraryItemsPresenter struct {
	SectionID string
	Items     []GenericMetadata
	RawData   interface{}
}

func (p *LibraryItemsPresenter) Title() string {
	return fmt.Sprintf("Items in %s", p.SectionID)
}

func (p *LibraryItemsPresenter) Headers() []string {
	return []string{"ID", "WATCHED", "TITLE", "TYPE", "YEAR", "DURATION", "RATING", "CONTENT", "GENRE"}
}

func (p *LibraryItemsPresenter) Rows() [][]string {
	var rows [][]string
	for _, item := range p.Items {
		row := append([]string{item.ID}, item.ToRow()...)
		rows = append(rows, row)
	}
	return rows
}

func (p *LibraryItemsPresenter) Raw() interface{} {
	return p.RawData
}

func (p *LibraryItemsPresenter) SortableColumns() []string {
	return []string{"id", "title", "year", "type"}
}

func (p *LibraryItemsPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].ID < p.Items[j].ID })
	case "title":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Title < p.Items[j].Title })
	case "year":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Year < p.Items[j].Year })
	case "type":
		sort.Slice(p.Items, func(i, j int) bool { return p.Items[i].Type < p.Items[j].Type })
	default:
		return false
	}
	return true
}

func (p *LibraryItemsPresenter) DefaultSort() string {
	return "title"
}
