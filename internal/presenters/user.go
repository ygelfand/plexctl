package presenters

import (
	"fmt"
	"sort"
	"strings"

	"github.com/LukeHagar/plexgo/models/operations"
)

type HomeUsersPresenter struct {
	Users []operations.HomeUser
}

func (p *HomeUsersPresenter) Title() string {
	return "Plex Home Users"
}

func (p *HomeUsersPresenter) Headers() []string {
	return []string{"ID", "TITLE", "USERNAME", "ADMIN", "PROTECTED"}
}

func (p *HomeUsersPresenter) Rows() [][]string {
	var rows [][]string
	for _, u := range p.Users {
		rows = append(rows, []string{
			fmt.Sprintf("%d", u.ID),
			u.Title,
			u.Username,
			fmt.Sprintf("%v", u.Admin),
			fmt.Sprintf("%v", u.Protected),
		})
	}
	return rows
}

func (p *HomeUsersPresenter) Raw() interface{} {
	return p.Users
}

func (p *HomeUsersPresenter) SortableColumns() []string {
	return []string{"id", "title", "username"}
}

func (p *HomeUsersPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Users, func(i, j int) bool {
			return p.Users[i].ID < p.Users[j].ID
		})
	case "title":
		sort.Slice(p.Users, func(i, j int) bool {
			return p.Users[i].Title < p.Users[j].Title
		})
	case "username":
		sort.Slice(p.Users, func(i, j int) bool {
			return p.Users[i].Username < p.Users[j].Username
		})
	default:
		return false
	}
	return true
}

func (p *HomeUsersPresenter) DefaultSort() string {
	return "title"
}

type UsersPresenter struct {
	Users []operations.User
}

func (p *UsersPresenter) Title() string {
	return "Plex Users"
}

func (p *UsersPresenter) Headers() []string {
	return []string{"ID", "TITLE", "USERNAME", "EMAIL"}
}

func (p *UsersPresenter) Rows() [][]string {
	var rows [][]string
	for _, u := range p.Users {
		rows = append(rows, []string{
			fmt.Sprintf("%d", u.ID),
			u.Title,
			u.Username,
			u.Email,
		})
	}
	return rows
}

func (p *UsersPresenter) Raw() interface{} {
	return p.Users
}

func (p *UsersPresenter) SortableColumns() []string {
	return []string{"id", "title", "username", "email"}
}

func (p *UsersPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Users, func(i, j int) bool {
			return p.Users[i].ID < p.Users[j].ID
		})
	case "title":
		sort.Slice(p.Users, func(i, j int) bool {
			return p.Users[i].Title < p.Users[j].Title
		})
	case "username":
		sort.Slice(p.Users, func(i, j int) bool {
			return p.Users[i].Username < p.Users[j].Username
		})
	case "email":
		sort.Slice(p.Users, func(i, j int) bool {
			return p.Users[i].Email < p.Users[j].Email
		})
	default:
		return false
	}
	return true
}

func (p *UsersPresenter) DefaultSort() string {
	return "username"
}
