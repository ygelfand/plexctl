package ui

import (
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
)

type HelpKey struct {
	Key  string
	Desc string
}

type HelpProvider interface {
	HelpKeys() []HelpKey
}

type SelectMediaMsg struct {
	Server    config.ServerTarget
	RatingKey string
	Type      string
	SectionID string
}

type ThemeChangedMsg struct {
	Theme tint.Tint
}

func (m ThemeChangedMsg) GetTheme() tint.Tint {
	return m.Theme
}

type JumpToDetailMsg struct {
	Server       config.ServerTarget
	SectionID    string
	RatingKey    string
	Type         string
	ReturnTabIdx int
}

type UserSelectionMsg struct {
	Users []operations.HomeUser
}

type SwitchUserMsg struct {
	User operations.HomeUser
	Pin  string
}

type InvalidPinMsg struct{}

type MediaPageMsg struct {
	ServerID  string
	SectionID string
	RequestID uint64
	Start     int
	Metadata  []components.Metadata
	Total     int
	Err       error
}

type RootChecker interface {
	IsAtRoot() bool
}

const AnnotationSkipServerCheck = "skip_server_check"

type Refreshable interface {
	Refresh() tea.Cmd
}

type PlayableProvider interface {
	GetSelectedMetadata() *components.Metadata
}

type RequestPlayMsg struct {
	Server    config.ServerTarget
	RatingKey string
	Output    config.VideoOutput
}

type ResumeChoiceMsg struct {
	Server   config.ServerTarget
	Metadata *components.Metadata
	Output   config.VideoOutput
}

func Ptr[T any](v T) *T {
	return &v
}

func PtrToBool(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

func PtrToString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ServerProvider identifies the server belonging to the current media selection.
type ServerProvider interface {
	MediaServer() config.ServerTarget
}
