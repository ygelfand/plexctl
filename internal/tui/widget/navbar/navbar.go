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
	cursor        int
	focused       bool
	theme         tint.Tint
	Width         int
	Height        int
	iconType      config.IconType
	nameFormat    config.LibraryNameFormat
	customIcons   map[string]string
	offset        int
	followActive  bool
	itemRows      []ui.Rect
	actionRows    []string
	actionTop     int
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
	n.cursor = index
	n.followActive = true
}

func (n *Navbar) SetFocused(focused bool) {
	if focused && !n.focused {
		n.cursor = n.activeFlatIdx
		n.followActive = true
	}
	n.focused = focused
}

func (n *Navbar) Cursor() int { return n.cursor }

func (n *Navbar) MoveCursor(delta int) {
	count := 0
	for _, section := range n.sections {
		count += len(section.Items)
	}
	n.cursor = max(0, min(count-1, n.cursor+delta))
	n.followActive = true
}

func (n *Navbar) Scroll(lines int) { n.offset = max(0, n.offset+lines) }

func (n *Navbar) ItemAt(mouse tea.MouseMsg) int {
	for i, rect := range n.itemRows {
		if _, ok := rect.Local(mouse); ok && mouse.Y < n.actionTop {
			return i
		}
	}
	return -1
}

func (n *Navbar) ActionAt(mouse tea.MouseMsg) string {
	mouse.Y -= n.actionTop
	return ui.ShortcutAt(strings.Join(n.actionRows, "\n"), mouse)
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
	n.itemRows = nil

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

	activeStyle := baseStyle.
		Foreground(accent).
		Bold(true).
		Background(lipgloss.Color("234")). // Subtle background for active item

		Border(lipgloss.Border{Left: "┃"}, false, false, false, true).
		BorderForeground(accent)
	cursorStyle := activeStyle.Foreground(n.theme.Black()).Background(accent)

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
			content := ui.Ellipsis(n.formatItem(item), max(1, n.Width-5))
			y := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, renderedLines...))
			n.itemRows = append(n.itemRows, ui.Rect{Y: y, Width: n.Width, Height: 1})
			if n.focused && flatIdx == n.cursor {
				renderedLines = append(renderedLines, cursorStyle.Render(content))
			} else if flatIdx == n.activeFlatIdx {
				renderedLines = append(renderedLines, activeStyle.Render(content))
			} else {
				renderedLines = append(renderedLines, baseStyle.Render(content))
			}
			flatIdx++
		}
	}

	lines := strings.Split(lipgloss.JoinVertical(lipgloss.Left, renderedLines...), "\n")
	n.actionRows = []string{"", " [/] Search", " [ctrl+s] Settings", " [L] Libraries", " [u] Switch user"}
	if n.Height < 12 {
		n.actionRows = nil
	}
	n.actionTop = max(1, n.Height-len(n.actionRows))
	followIdx := n.activeFlatIdx
	if n.focused {
		followIdx = n.cursor
	}
	if n.followActive && followIdx >= 0 && followIdx < len(n.itemRows) {
		y := n.itemRows[followIdx].Y
		if y < n.offset {
			n.offset = y
		} else if y >= n.offset+n.actionTop {
			n.offset = y - n.actionTop + 1
		}
		n.followActive = false
	}
	n.offset = min(n.offset, max(0, len(lines)-n.actionTop))
	for i := range n.itemRows {
		n.itemRows[i].Y -= n.offset
	}
	visible := append([]string(nil), lines[n.offset:min(len(lines), n.offset+n.actionTop)]...)
	for len(visible) < n.actionTop {
		visible = append(visible, "")
	}
	visible = append(visible, n.actionRows...)
	sidebarContent := strings.Join(visible, "\n")
	border := n.theme.BrightBlack()
	if n.focused {
		border = accent
	}

	// Container style with right border and fixed height
	return lipgloss.NewStyle().
		Width(n.Width).
		Height(n.Height).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(border).
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
