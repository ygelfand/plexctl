package presenters

import (
	"sort"
	"strings"

	"github.com/LukeHagar/plexgo/models/operations"
)

// SessionListPresenter formats active playback sessions
type SessionListPresenter struct {
	Metadata []operations.Metadata
}

func (p *SessionListPresenter) Title() string {
	return "Active Playback Sessions"
}

func (p *SessionListPresenter) Headers() []string {
	return []string{"USER", "PLAYER", "TITLE", "TYPE", "STATE"}
}

func (p *SessionListPresenter) Rows() [][]string {
	var rows [][]string
	for _, meta := range p.Metadata {
		user := ""
		if meta.User != nil && meta.User.Title != nil {
			user = *meta.User.Title
		}
		player := ""
		if meta.Player != nil && meta.Player.Title != nil {
			player = *meta.Player.Title
		}
		state := ""
		if meta.Player != nil && meta.Player.State != nil {
			state = *meta.Player.State
		}

		rows = append(rows, []string{user, player, meta.Title, meta.Type, state})
	}
	return rows
}

func (p *SessionListPresenter) Raw() interface{} {
	return p.Metadata
}

func (p *SessionListPresenter) SortableColumns() []string {
	return []string{"user", "title", "type"}
}

func (p *SessionListPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "user":
		sort.Slice(p.Metadata, func(i, j int) bool {
			userI := ""
			if p.Metadata[i].User != nil && p.Metadata[i].User.Title != nil {
				userI = *p.Metadata[i].User.Title
			}
			userJ := ""
			if p.Metadata[j].User != nil && p.Metadata[j].User.Title != nil {
				userJ = *p.Metadata[j].User.Title
			}
			return userI < userJ
		})
	case "title":
		sort.Slice(p.Metadata, func(i, j int) bool {
			return p.Metadata[i].Title < p.Metadata[j].Title
		})
	case "type":
		sort.Slice(p.Metadata, func(i, j int) bool {
			return p.Metadata[i].Type < p.Metadata[j].Type
		})
	default:
		return false
	}
	return true
}

func (p *SessionListPresenter) DefaultSort() string {
	return "user"
}
