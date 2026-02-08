package presenters

import (
	"fmt"
	"sort"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
)

type CollectionsPresenter struct {
	SectionID   string
	Collections []components.Metadata
	RawData     interface{}
}

func (p *CollectionsPresenter) Title() string {
	return fmt.Sprintf("Collections in Library %s", p.SectionID)
}

func (p *CollectionsPresenter) Headers() []string {
	return []string{"ID", "TITLE", "ITEMS"}
}

func (p *CollectionsPresenter) Rows() [][]string {
	var rows [][]string
	for _, c := range p.Collections {
		id := ""
		if c.RatingKey != nil {
			id = *c.RatingKey
		}
		count := ""
		if c.ChildCount != nil {
			count = fmt.Sprintf("%d", *c.ChildCount)
		}
		rows = append(rows, []string{id, c.Title, count})
	}
	return rows
}

func (p *CollectionsPresenter) Raw() interface{} {
	return p.RawData
}

func (p *CollectionsPresenter) SortableColumns() []string {
	return []string{"id", "title", "items"}
}

func (p *CollectionsPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Collections, func(i, j int) bool {
			idI := ""
			if p.Collections[i].RatingKey != nil {
				idI = *p.Collections[i].RatingKey
			}
			idJ := ""
			if p.Collections[j].RatingKey != nil {
				idJ = *p.Collections[j].RatingKey
			}
			return idI < idJ
		})
	case "title":
		sort.Slice(p.Collections, func(i, j int) bool { return p.Collections[i].Title < p.Collections[j].Title })
	case "items":
		sort.Slice(p.Collections, func(i, j int) bool {
			cI := 0
			if p.Collections[i].ChildCount != nil {
				cI = *p.Collections[i].ChildCount
			}
			cJ := 0
			if p.Collections[j].ChildCount != nil {
				cJ = *p.Collections[j].ChildCount
			}
			return cI < cJ
		})
	default:
		return false
	}
	return true
}

func (p *CollectionsPresenter) DefaultSort() string {
	return "title"
}
