package presenters

import (
	"fmt"
	"sort"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/ui"
)

// ServerListPresenter formats a list of Plex servers
type ServerListPresenter struct {
	Devices []components.PlexDevice
}

func (p *ServerListPresenter) Title() string {
	return "Available Plex Servers"
}

func (p *ServerListPresenter) Headers() []string {
	return []string{"ID", "NAME", "PRODUCT", "VERSION", "PLATFORM", "LAST SEEN"}
}

func (p *ServerListPresenter) Rows() [][]string {
	var rows [][]string
	for _, d := range p.Devices {
		rows = append(rows, []string{
			d.ClientIdentifier,
			d.Name,
			d.Product,
			d.ProductVersion,
			ui.PtrToString(d.Platform),
			d.LastSeenAt.Format("2006-01-02 15:04"),
		})
	}
	return rows
}

func (p *ServerListPresenter) Raw() interface{} {
	return p.Devices
}

func (p *ServerListPresenter) SortableColumns() []string {
	return []string{"id", "name", "product"}
}

func (p *ServerListPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Devices, func(i, j int) bool { return p.Devices[i].ClientIdentifier < p.Devices[j].ClientIdentifier })
	case "name":
		sort.Slice(p.Devices, func(i, j int) bool { return p.Devices[i].Name < p.Devices[j].Name })
	case "product":
		sort.Slice(p.Devices, func(i, j int) bool { return p.Devices[i].Product < p.Devices[j].Product })
	default:
		return false
	}
	return true
}

func (p *ServerListPresenter) DefaultSort() string {
	return "name"
}

type ServerIdentityPresenter struct {
	Container *operations.GetIdentityMediaContainer
}

func (p *ServerIdentityPresenter) Title() string {
	return "Server Identity"
}

func (p *ServerIdentityPresenter) Headers() []string {
	return []string{"MACHINE ID", "VERSION", "CLAIMED"}
}

func (p *ServerIdentityPresenter) Rows() [][]string {
	c := p.Container
	return [][]string{{
		ui.PtrToString(c.MachineIdentifier),
		ui.PtrToString(c.Version),
		fmt.Sprintf("%v", ui.PtrToBool(c.Claimed)),
	}}
}

func (p *ServerIdentityPresenter) Raw() interface{} {
	return p.Container
}

func (p *ServerIdentityPresenter) SortableColumns() []string {
	return nil
}

func (p *ServerIdentityPresenter) SortBy(column string) bool {
	return false
}

func (p *ServerIdentityPresenter) DefaultSort() string {
	return ""
}

type DevicesPresenter struct {
	Devices []components.PlexDevice
}

func (p *DevicesPresenter) Title() string {
	return "Plex Account Devices"
}

func (p *DevicesPresenter) Headers() []string {
	return []string{"ID", "NAME", "PRODUCT", "OS", "PLATFORM", "LAST SEEN"}
}

func (p *DevicesPresenter) Rows() [][]string {
	var rows [][]string
	for _, d := range p.Devices {
		rows = append(rows, []string{
			d.ClientIdentifier,
			d.Name,
			d.Product,
			ui.PtrToString(d.Device),
			ui.PtrToString(d.Platform),
			d.LastSeenAt.Format("2006-01-02 15:04"),
		})
	}
	return rows
}

func (p *DevicesPresenter) Raw() interface{} {
	return p.Devices
}

func (p *DevicesPresenter) SortableColumns() []string {
	return []string{"id", "name", "product", "lastseen"}
}

func (p *DevicesPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Devices, func(i, j int) bool { return p.Devices[i].ClientIdentifier < p.Devices[j].ClientIdentifier })
	case "name":
		sort.Slice(p.Devices, func(i, j int) bool { return p.Devices[i].Name < p.Devices[j].Name })
	case "product":
		sort.Slice(p.Devices, func(i, j int) bool { return p.Devices[i].Product < p.Devices[j].Product })
	case "lastseen":
		sort.Slice(p.Devices, func(i, j int) bool {
			return p.Devices[i].LastSeenAt.Before(p.Devices[j].LastSeenAt)
		})
	default:
		return false
	}
	return true
}

func (p *DevicesPresenter) DefaultSort() string {
	return "name"
}

type SessionMetadata struct {
	ID     string
	User   string
	Player string
	Title  string
	State  string
}

type SessionsPresenter struct {
	Sessions []SessionMetadata
	RawData  interface{}
}

func (p *SessionsPresenter) Title() string {
	return "Active Sessions"
}

func (p *SessionsPresenter) Headers() []string {
	return []string{"ID", "USER", "PLAYER", "TITLE", "STATE"}
}

func (p *SessionsPresenter) Rows() [][]string {
	var rows [][]string
	for _, s := range p.Sessions {
		rows = append(rows, []string{s.ID, s.User, s.Player, s.Title, s.State})
	}
	return rows
}

func (p *SessionsPresenter) Raw() interface{} {
	return p.RawData
}

func (p *SessionsPresenter) SortableColumns() []string {
	return []string{"id", "user", "player", "title", "state"}
}

func (p *SessionsPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "id":
		sort.Slice(p.Sessions, func(i, j int) bool { return p.Sessions[i].ID < p.Sessions[j].ID })
	case "user":
		sort.Slice(p.Sessions, func(i, j int) bool { return p.Sessions[i].User < p.Sessions[j].User })
	case "player":
		sort.Slice(p.Sessions, func(i, j int) bool { return p.Sessions[i].Player < p.Sessions[j].Player })
	case "title":
		sort.Slice(p.Sessions, func(i, j int) bool { return p.Sessions[i].Title < p.Sessions[j].Title })
	case "state":
		sort.Slice(p.Sessions, func(i, j int) bool { return p.Sessions[i].State < p.Sessions[j].State })
	default:
		return false
	}
	return true
}

func (p *SessionsPresenter) DefaultSort() string {
	return "title"
}
