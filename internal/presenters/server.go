package presenters

import (
	"sort"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
)

// ServerListPresenter formats a list of Plex servers
type ServerListPresenter struct {
	Devices []components.PlexDevice
}

func (p *ServerListPresenter) Title() string {
	return "Available Plex Servers"
}

func (p *ServerListPresenter) Headers() []string {
	return []string{"NAME", "PRODUCT", "VERSION", "URI", "LOCAL"}
}

func (p *ServerListPresenter) Rows() [][]string {
	var rows [][]string
	for _, device := range p.Devices {
		if !strings.Contains(device.Provides, "server") {
			continue
		}
		for _, conn := range device.Connections {
			local := "false"
			if conn.Local {
				local = "true"
			}
			rows = append(rows, []string{device.Name, device.Product, device.ProductVersion, conn.URI, local})
		}
	}
	return rows
}

func (p *ServerListPresenter) Raw() interface{} {
	return p.Devices
}

func (p *ServerListPresenter) SortableColumns() []string {
	return []string{"name", "product", "version"}
}

func (p *ServerListPresenter) SortBy(column string) bool {
	switch strings.ToLower(column) {
	case "name":
		sort.Slice(p.Devices, func(i, j int) bool {
			return p.Devices[i].Name < p.Devices[j].Name
		})
	case "product":
		sort.Slice(p.Devices, func(i, j int) bool {
			return p.Devices[i].Product < p.Devices[j].Product
		})
	case "version":
		sort.Slice(p.Devices, func(i, j int) bool {
			return p.Devices[i].ProductVersion < p.Devices[j].ProductVersion
		})
	default:
		return false
	}
	return true
}

func (p *ServerListPresenter) DefaultSort() string {
	return "name"
}

// ServerIdentityPresenter formats the server identity
type ServerIdentityPresenter struct {
	Container *operations.GetIdentityMediaContainer
}

func (p *ServerIdentityPresenter) Title() string {
	return "Plex Media Server Identity"
}

func (p *ServerIdentityPresenter) Headers() []string {
	return []string{"Version", "Machine ID", "Claimed"}
}

func (p *ServerIdentityPresenter) Rows() [][]string {
	version := ""
	if p.Container.Version != nil {
		version = *p.Container.Version
	}
	machineID := ""
	if p.Container.MachineIdentifier != nil {
		machineID = *p.Container.MachineIdentifier
	}
	claimed := "false"
	if p.Container.Claimed != nil && *p.Container.Claimed {
		claimed = "true"
	}

	return [][]string{{version, machineID, claimed}}
}

func (p *ServerIdentityPresenter) Raw() interface{} {
	return p.Container
}

func (p *ServerIdentityPresenter) SortableColumns() []string {
	return []string{}
}

func (p *ServerIdentityPresenter) SortBy(column string) bool {
	return false
}

func (p *ServerIdentityPresenter) DefaultSort() string {
	return ""
}
