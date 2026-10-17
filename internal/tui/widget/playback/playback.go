package playback

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/termgraphics"
	"github.com/ygelfand/plexctl/internal/ui"
)

type choice struct {
	label  string
	output config.VideoOutput
}

var choices = []choice{
	{"Automatic", config.VideoAuto},
	{"Kitty graphics", config.VideoKitty},
	{"Sixel graphics", config.VideoSixel},
	{"Text (compatible terminals)", config.VideoTCT},
}

type Model struct {
	server    config.ServerTarget
	ratingKey string
	title     string
	theme     tint.Tint
	choice    int
	width     int
	choices   []choice
	music     bool
}

func New(ratingKey, title string, theme tint.Tint, servers ...config.ServerTarget) *Model {
	m := &Model{server: config.MediaServer(servers...), ratingKey: ratingKey, title: title, theme: theme, width: 58, choices: choices}
	for i, choice := range choices {
		if choice.output == config.Get().TerminalVideo {
			m.choice = i
		}
	}
	return m
}

func NewMusic(ratingKey, title string, theme tint.Tint, servers ...config.ServerTarget) *Model {
	m := &Model{server: config.MediaServer(servers...), ratingKey: ratingKey, title: title, theme: theme, width: 58, music: true,
		choices: []choice{{"In this terminal · audio + visualizer", config.AudioTerminal},
			{"Separate player window", config.VideoWindow}}}
	if config.Get().MusicOutput() == config.VideoWindow {
		m.choice = 1
	}
	return m
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) ActionRows() int { return 1 }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if delta := ui.WheelDelta(msg); delta != 0 {
			m.choice = max(0, min(len(m.choices)-1, m.choice+delta))
		} else if ui.LeftClick(msg) {
			for i, choice := range m.choices {
				if ui.LabelAt(m.View(), msg, choice.label) {
					m.choice = i
					return m.Update(ui.KeyPress("enter"))
				}
			}
		}
	case tea.WindowSizeMsg:
		m.width = max(20, min(58, msg.Width-8))
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.choice = (m.choice + len(m.choices) - 1) % len(m.choices)
		case "down", "j":
			m.choice = (m.choice + 1) % len(m.choices)
		case "esc", "q":
			return nil, nil
		case "enter":
			output := m.choices[m.choice].output
			return nil, func() tea.Msg {
				return ui.RequestPlayMsg{Server: m.server, RatingKey: m.ratingKey, Output: output}
			}
		}
	}
	return m, nil
}

func (m *Model) View() string {
	accent := lipgloss.NewStyle().Foreground(ui.Accent(m.theme)).Bold(true)
	lines := []string{accent.Render("Play in terminal"), ui.Ellipsis(m.title, m.width-4), ""}
	if m.music {
		lines[0] = accent.Render("Music player")
	}
	for i, choice := range m.choices {
		line := "  " + choice.label
		if m.choice == i {
			line = accent.Render("> " + choice.label)
		}
		lines = append(lines, line)
	}
	caps := termgraphics.Current()
	detected := "Choose a renderer supported by your terminal."
	switch {
	case caps.Kitty:
		detected = "Detected Kitty graphics."
		if caps.NativePosters() {
			detected += " Native posters enabled."
		}
	case caps.Sixel:
		detected = "Detected Sixel graphics."
	case caps.Responded:
		name := caps.Name
		if name == "" {
			name = "This terminal"
		}
		detected = fmt.Sprintf("%s: text output available. Open Kitty for graphics.", name)
	}
	lines = append(lines, "", lipgloss.NewStyle().Width(m.width-4).Render(detected), "",
		"Kitty: clickable controls, Tab/Enter, h to hide",
		"Space / click: pause  Arrows / wheel: seek", "Right click / q: return", "", "[enter] Play | [esc] Cancel")
	if m.music {
		lines = lines[:3+len(m.choices)]
		lines = append(lines, "", "Browse your library while listening.",
			"Space: pause · [ / ]: seek · + / -: volume",
			"m: mute · z: visualizer · x: stop", "", "[enter] Play | [esc] Cancel")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.Accent(m.theme)).Padding(1, 2).Width(m.width).
		Render(strings.Join(lines, "\n"))
}
