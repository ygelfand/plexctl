package view

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
)

// DataLoadedMsg follows its originating view even if the user changes tabs.
// Replaced views and superseded refreshes cannot consume the result.
type DataLoadedMsg struct {
	owner tea.Model
	id    uint64
	data  any
	err   error
}

func (m DataLoadedMsg) Owns(model tea.Model) bool { return m.owner == model }

type viewRequest struct {
	server config.ServerTarget
	id     uint64
}

func newViewRequest() viewRequest {
	return viewRequest{server: config.Get().ServerTarget()}
}

func (r *viewRequest) start(owner tea.Model, load func(context.Context) (any, error)) tea.Cmd {
	r.id++
	id, server := r.id, r.server
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(server.Context(context.Background()), 30*time.Second)
		defer cancel()
		data, err := load(ctx)
		return DataLoadedMsg{owner: owner, id: id, data: data, err: err}
	}
}

func (r *viewRequest) accepts(owner tea.Model, msg DataLoadedMsg) bool {
	return msg.Owns(owner) && msg.id == r.id
}
