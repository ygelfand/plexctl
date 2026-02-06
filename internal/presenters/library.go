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
	return []string{"ID", "TITLE", "TYPE", "AGENT"}
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
		rows = append(rows, []string{id, title, string(dir.Type), agent})
	}
	return rows
}

func (p *LibraryListPresenter) Raw() interface{} {
	return p.Directories
}

func (p *LibraryListPresenter) SortableColumns() []string {
	return []string{"id", "title", "type"}
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
	Metadata  []components.Metadata
}

func (p *LibraryItemsPresenter) Title() string {
	return fmt.Sprintf("Items in Library %s", p.SectionID)
}

func (p *LibraryItemsPresenter) Headers() []string {
	return []string{"ID", "TITLE", "TYPE", "YEAR"}
}

func (p *LibraryItemsPresenter) Rows() [][]string {
	var rows [][]string
	for _, meta := range p.Metadata {
		id := ""
		if meta.RatingKey != nil {
			id = *meta.RatingKey
		}
		year := ""
		if meta.Year != nil {
			year = fmt.Sprintf("%d", *meta.Year)
		}
		rows = append(rows, []string{id, meta.Title, meta.Type, year})
	}
	return rows
}

func (p *LibraryItemsPresenter) Raw() interface{} {
	return p.Metadata
}

func (p *LibraryItemsPresenter) SortableColumns() []string {
	return []string{"id", "title", "year"}
}

func (p *LibraryItemsPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Metadata, func(i, j int) bool {
			idI := ""
			if p.Metadata[i].RatingKey != nil {
				idI = *p.Metadata[i].RatingKey
			}
			idJ := ""
			if p.Metadata[j].RatingKey != nil {
				idJ = *p.Metadata[j].RatingKey
			}
			return idI < idJ
		})
	case "title":
		sort.Slice(p.Metadata, func(i, j int) bool {
			return p.Metadata[i].Title < p.Metadata[j].Title
		})
	case "year":
		sort.Slice(p.Metadata, func(i, j int) bool {
			yI := 0
			if p.Metadata[i].Year != nil {
				yI = *p.Metadata[i].Year
			}
			yJ := 0
			if p.Metadata[j].Year != nil {
				yJ = *p.Metadata[j].Year
			}
			return yI < yJ
		})
	default:
		return false
	}
	return true
}

func (p *LibraryItemsPresenter) DefaultSort() string {
	return "title"
}
