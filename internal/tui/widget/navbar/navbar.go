package navbar

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyokomi/emoji/v2"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

type NavItem struct {
	ID    string
	Title string
	Type  string
}

type NavSection struct {
	Title string
	Items []NavItem
}

type Navbar struct {
	sections      []NavSection
	activeFlatIdx int
	theme         tint.Tint
	Width         int
	Height        int
	iconType      config.IconType
	nameFormat    config.LibraryNameFormat
	customIcons   map[string]string
}

func NewNavbar(sections []NavSection, theme tint.Tint) *Navbar {
	cfg := config.Get()

	iconType := cfg.IconType
	if iconType == "" {
		iconType = config.IconTypeEmoji
	}

	nameFormat := cfg.LibraryNameFormat
	if nameFormat == "" {
		nameFormat = config.LibraryNameFormatIconName
	}

	return &Navbar{
		sections:    sections,
		theme:       theme,
		iconType:    iconType,
		nameFormat:  nameFormat,
		customIcons: make(map[string]string),
		Width:       ui.SidebarWidth,
	}
}

func (n *Navbar) SetActive(index int) {
	n.activeFlatIdx = index
}

func (n *Navbar) SetCustomIcons(icons map[string]string) {
	n.customIcons = icons
}

func (n *Navbar) Init() tea.Cmd {
	return nil
}

func (n *Navbar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.ThemeChangedMsg:
		n.theme = msg.Theme
	}
	return n, nil
}

func (n *Navbar) View() string {

	var renderedLines []string

	accent := ui.Accent(n.theme)

	// Header / Logo area

	logoStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true).
		Margin(1, 0, 0, 2)

	renderedLines = append(renderedLines, logoStyle.Render("󰀚 PLEXCTL"))

	renderedLines = append(renderedLines, lipgloss.NewStyle().
		Foreground(n.theme.BrightBlack()).
		Margin(0, 0, 1, 2).
		Render(strings.Repeat("─", n.Width-4)))

	// Style for items

	baseStyle := lipgloss.NewStyle().
		Width(n.Width-3). // Account for padding and potential border

		Padding(0, 1)

	activeStyle := baseStyle.Copy().
		Foreground(accent).
		Bold(true).
		Background(lipgloss.Color("234")). // Subtle background for active item

		Border(lipgloss.Border{Left: "┃"}, false, false, false, true).
		BorderForeground(accent)

	sectionHeaderStyle := lipgloss.NewStyle().
		Foreground(n.theme.BrightBlack()).
		Bold(true).
		Margin(1, 0, 0, 1)

	flatIdx := 0
	for _, section := range n.sections {
		if section.Title != "" {
			renderedLines = append(renderedLines, sectionHeaderStyle.Render(" "+strings.ToUpper(section.Title)))
		}

		for _, item := range section.Items {
			content := n.formatItem(item)
			if flatIdx == n.activeFlatIdx {
				renderedLines = append(renderedLines, activeStyle.Render(content))
			} else {
				renderedLines = append(renderedLines, baseStyle.Render(content))
			}
			flatIdx++
		}
	}

	sidebarContent := lipgloss.JoinVertical(lipgloss.Left, renderedLines...)

	// Container style with right border and fixed height
	return lipgloss.NewStyle().
		Width(n.Width).
		Height(n.Height).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(n.theme.BrightBlack()).
		Render(sidebarContent)
}

func (n *Navbar) formatItem(item NavItem) string {
	icon := n.getIcon(item)
	name := item.Title

	switch n.nameFormat {
	case config.LibraryNameFormatIconOnly:
		return icon
	case config.LibraryNameFormatIconName:
		return fmt.Sprintf("%s %s", icon, name)
	case config.LibraryNameFormatNameIcon:
		return fmt.Sprintf("%s %s", name, icon)
	case config.LibraryNameFormatName:
		return name
	default:
		return fmt.Sprintf("%s %s", icon, name)
	}
}

func (n *Navbar) getIcon(item NavItem) string {
	if custom, ok := n.customIcons[item.ID]; ok && custom != "" {
		return emoji.Sprint(custom)
	}

	switch n.iconType {
	case config.IconTypeASCII:
		return n.getAsciiIcon(item.Type)
	case config.IconTypeNerdFonts:
		return n.getNerdFontIcon(item.Type)
	case config.IconTypeEmoji:
		fallthrough
	default:
		return emoji.Sprint(n.getEmojiIcon(item.Type))
	}
}

func (n *Navbar) getEmojiIcon(libType string) string {
	switch libType {
	case "home":
		return "🏠"
	case "sessions":
		return "📽"
	case "history":
		return "📜"
	case "tasks":
		return "⚒"
	case "search_status":
		return "🔍"
	case "movie":
		return "🎬"
	case "show":
		return "📺"
	case "artist":
		return "🎵"
	case "photo":
		return "📷"
	default:
		return "📁"
	}
}

func (n *Navbar) getAsciiIcon(libType string) string {
	switch libType {
	case "home":
		return "H"
	case "sessions":
		return "S"
	case "history":
		return "L"
	case "tasks":
		return "T"
	case "search_status":
		return "Q"
	case "movie":
		return "M"
	case "show":
		return "T"
	case "artist":
		return "A"
	case "photo":
		return "P"
	default:
		return "D"
	}
}

func (n *Navbar) getNerdFontIcon(libType string) string {
	switch libType {
	case "home":
		return ""
	case "sessions":
		return ""
	case "history":
		return ""
	case "tasks":
		return ""
	case "search_status":
		return ""
	case "movie":
		return "󰿎"
	case "show":
		return "󰿟"
	case "artist":
		return "󰎆"
	case "photo":
		return "󰄄"
	default:
		return ""
	}
}
