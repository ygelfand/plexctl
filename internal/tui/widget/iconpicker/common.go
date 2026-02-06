package iconpicker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ygelfand/plexctl/internal/config"
)

type Mode int

const (
	GridMode Mode = iota
	SearchMode
	ManualMode
)

type IconInfo struct {
	Char string
	Name string
}

// IconProvider defines the interface for different icon sets
type IconProvider interface {
	GetIconList() []IconInfo
	GetCommonIcons() []IconInfo
	CanSearch() bool
}

type IconPicker struct {
	Mode     Mode
	IconType config.IconType
	Provider IconProvider
	Grid     [][]IconInfo
	Row, Col int

	Search    textinput.Model
	Results   []IconInfo
	ResultIdx int

	Manual   textinput.Model
	Selected string
	iconList []IconInfo
}

func NewIconPicker(iconType config.IconType) *IconPicker {
	search := textinput.New()
	search.Placeholder = "Search icons..."
	search.Focus()

	manual := textinput.New()
	manual.Placeholder = "Type or paste icon"
	manual.CharLimit = 16

	p := &IconPicker{
		IconType: iconType,
		Search:   search,
		Manual:   manual,
	}

	switch iconType {
	case config.IconTypeASCII:
		p.Provider = &ASCIIProvider{}
	case config.IconTypeNerdFonts:
		p.Provider = &NerdFontProvider{}
	case config.IconTypeEmoji:
		fallthrough
	default:
		p.Provider = &EmojiProvider{}
	}

	p.iconList = p.Provider.GetIconList()
	p.Grid = p.generateGrid()
	return p
}

func (p *IconPicker) generateGrid() [][]IconInfo {
	var grid [][]IconInfo
	row := []IconInfo{}
	common := p.Provider.GetCommonIcons()

	for _, icon := range common {
		row = append(row, icon)
		if len(row) == 10 {
			grid = append(grid, row)
			row = []IconInfo{}
		}
	}
	if len(row) > 0 {
		grid = append(grid, row)
	}
	return grid
}

func (p *IconPicker) Init() tea.Cmd {
	return nil
}

func (p *IconPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			p.Mode = (p.Mode + 1) % 3
			if p.Mode == SearchMode && !p.Provider.CanSearch() {
				p.Mode = (p.Mode + 1) % 3
			}

			switch p.Mode {
			case SearchMode:
				p.Search.Focus()
			case ManualMode:
				p.Manual.Reset()
				p.Manual.Focus()
			default:
				p.Search.Blur()
				p.Manual.Blur()
			}
			return p, nil
		case "/":
			if p.Provider.CanSearch() {
				p.Mode = SearchMode
				p.Search.Focus()
			}
			return p, nil

		case "enter":
			if sel := p.selectIcon(); sel != "" {
				p.Selected = sel
				return p, nil
			}
		}

		p.handleKeys(msg)
	}

	if p.Mode == SearchMode {
		oldVal := p.Search.Value()
		p.Search, cmd = p.Search.Update(msg)
		if p.Search.Value() != oldVal {
			p.filter()
		}
		return p, cmd
	}
	if p.Mode == ManualMode {
		p.Manual, cmd = p.Manual.Update(msg)
		return p, cmd
	}

	return p, nil
}

func (p *IconPicker) handleKeys(msg tea.KeyMsg) {
	switch p.Mode {
	case GridMode:
		switch msg.String() {
		case "left":
			p.Col = max(0, p.Col-1)
		case "right":
			p.Col = min(len(p.Grid[p.Row])-1, p.Col+1)
		case "up":
			p.Row = max(0, p.Row-1)
		case "down":
			p.Row = min(len(p.Grid)-1, p.Row+1)
			if p.Row < len(p.Grid) {
				p.Col = min(p.Col, len(p.Grid[p.Row])-1)
			}
		}
	case SearchMode:
		switch msg.String() {
		case "up":
			p.ResultIdx = max(0, p.ResultIdx-1)
		case "down":
			p.ResultIdx = min(len(p.Results)-1, p.ResultIdx+1)
		}
	}
}

func (p *IconPicker) filter() {
	q := strings.ToLower(p.Search.Value())
	p.Results = nil
	p.ResultIdx = 0

	if q == "" {
		return
	}

	for _, e := range p.iconList {
		if strings.Contains(strings.ToLower(e.Name), q) {
			p.Results = append(p.Results, e)
			if len(p.Results) >= 10 {
				break
			}
		}
	}
}

func (p *IconPicker) selectIcon() string {
	switch p.Mode {
	case GridMode:
		if p.Row < len(p.Grid) && p.Col < len(p.Grid[p.Row]) {
			return p.Grid[p.Row][p.Col].Char
		}
	case SearchMode:
		if len(p.Results) > 0 && p.ResultIdx < len(p.Results) {
			return p.Results[p.ResultIdx].Char
		}
	case ManualMode:
		return strings.TrimSpace(p.Manual.Value())
	}
	return ""
}

func (p *IconPicker) View() string {
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true).MarginBottom(1)

	var modeTitle string
	var content string

	switch p.Mode {
	case GridMode:
		modeTitle = "Grid"
		content = p.gridView()
	case SearchMode:
		modeTitle = "Search"
		content = p.searchView()
	case ManualMode:
		modeTitle = "Manual"
		content = p.manualView()
	}

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).MarginTop(1).Render("tab: switch mode | /: search | enter: select")

	return lipgloss.NewStyle().Width(60).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Pick Icon: "+modeTitle),
			content,
			help,
		),
	)
}

func (p *IconPicker) gridView() string {
	var rows []string
	for r, row := range p.Grid {
		var cells []string
		for c, e := range row {
			style := lipgloss.NewStyle().Padding(0, 1)
			if r == p.Row && c == p.Col {
				style = style.Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Bold(true)
			}
			cells = append(cells, style.Render(e.Char))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (p *IconPicker) searchView() string {
	var b strings.Builder
	b.WriteString(p.Search.View() + "\n\n")

	if len(p.Results) == 0 {
		if p.Search.Value() != "" {
			b.WriteString(" No results found.")
		} else {
			b.WriteString(" Type to search...")
		}
	} else {
		for i, e := range p.Results {
			prefix := "  "
			if i == p.ResultIdx {
				prefix = "> "
				b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render(fmt.Sprintf("%s %s %s", prefix, e.Char, e.Name)) + "\n")
			} else {
				b.WriteString(fmt.Sprintf("%s %s %s\n", prefix, e.Char, e.Name))
			}
		}
	}

	return b.String()
}

func (p *IconPicker) manualView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		"Type or paste an icon",
		"",
		p.Manual.View(),
	)
}
