package config

import (
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kyokomi/emoji/v2"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/widget/iconpicker"
	"github.com/ygelfand/plexctl/internal/ui"
)

type libConfigItem struct {
	id    string
	title string
	icon  string
}

func (i libConfigItem) Title() string {
	if i.icon != "" {
		return i.icon + " " + i.title
	}
	return i.title
}
func (i libConfigItem) Description() string { return "ID: " + i.id }
func (i libConfigItem) FilterValue() string { return i.title }

type ConfigOverlayModel struct {
	lists           [2]list.Model
	picker          *iconpicker.IconPicker
	allLibs         []plex.LibraryInfo
	focusSide       int // 0: Hidden, 1: Active
	isPicking       bool
	currentIconType config.IconType
}

type ConfigFinishedMsg struct {
	IconType config.IconType
}

func NewConfigOverlayModel(allLibs []plex.LibraryInfo) *ConfigOverlayModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	cfg := config.Get()
	iconType := cfg.IconType
	if iconType == "" {
		iconType = config.IconTypeEmoji
	}

	m := &ConfigOverlayModel{
		allLibs:         allLibs,
		currentIconType: iconType,
		focusSide:       1, // Start on Active side
	}

	titles := []string{"Hidden", "Active"}
	for i := range m.lists {
		m.lists[i] = list.New(nil, delegate, 30, 15)
		m.lists[i].Title = titles[i]
		m.lists[i].SetShowStatusBar(false)
		m.lists[i].SetFilteringEnabled(false)
		m.lists[i].KeyMap.CursorUp.SetKeys("up")
		m.lists[i].KeyMap.CursorDown.SetKeys("down")
		m.lists[i].KeyMap.PrevPage.SetKeys("pgup")
		m.lists[i].KeyMap.NextPage.SetKeys("pgdown")
	}

	m.updateColumns()
	return m
}

func (m *ConfigOverlayModel) updateColumns() {
	cfg := config.Get()
	_, server, _ := cfg.GetActiveServer()

	var hiddenItems []list.Item
	var activeItems []list.Item

	for _, lib := range m.allLibs {
		opts, ok := server.Libraries.Settings[lib.ID]
		icon := ""
		isHidden := false
		if ok {
			isHidden = opts.Hidden
			icon = opts.GetIcon(m.currentIconType)
			if icon != "" && m.currentIconType == config.IconTypeEmoji {
				icon = emoji.Sprint(icon)
			}
		}

		item := libConfigItem{id: lib.ID, title: lib.Title, icon: icon}

		if isHidden {
			hiddenItems = append(hiddenItems, item)
		} else {
			activeItems = append(activeItems, item)
		}
	}

	// Respect configured order for active items
	if len(server.Libraries.Order) > 0 {
		ordered := make([]list.Item, 0, len(activeItems))
		for _, id := range server.Libraries.Order {
			for _, item := range activeItems {
				if item.(libConfigItem).id == id {
					ordered = append(ordered, item)
					break
				}
			}
		}
		// Add remaining items not in Order list
		for _, item := range activeItems {
			found := false
			for _, o := range ordered {
				if o.(libConfigItem).id == item.(libConfigItem).id {
					found = true
					break
				}
			}
			if !found {
				ordered = append(ordered, item)
			}
		}
		activeItems = ordered
	}

	m.lists[0].SetItems(hiddenItems)
	m.lists[1].SetItems(activeItems)
}

func (m *ConfigOverlayModel) Init() tea.Cmd {
	return nil
}

func (m *ConfigOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.isPicking {
		if m.picker == nil {
			m.picker = iconpicker.NewIconPicker(m.currentIconType)
		}

		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
			m.isPicking = false
			m.picker = nil
			return m, nil
		}

		_, cmd = m.picker.Update(msg)

		if m.picker.Selected != "" {
			iconChar := strings.TrimSpace(m.picker.Selected)
			item := m.lists[m.focusSide].SelectedItem().(libConfigItem)
			cfg := config.Get()
			id, srv, ok := cfg.GetActiveServer()
			if ok {
				if srv.Libraries.Settings == nil {
					srv.Libraries.Settings = make(map[string]config.LibraryOptions)
				}
				opts := srv.Libraries.Settings[item.id]

				switch m.currentIconType {
				case config.IconTypeEmoji:
					opts.IconEmoji = iconChar
				case config.IconTypeNerdFonts:
					opts.IconNF = iconChar
				case config.IconTypeASCII:
					opts.IconASCII = iconChar
				}

				srv.Libraries.Settings[item.id] = opts

				if !slices.Contains(srv.Libraries.Order, item.id) {
					srv.Libraries.Order = append(srv.Libraries.Order, item.id)
				}

				cfg.Servers[id] = srv
				_ = cfg.Save()
			}

			m.isPicking = false
			m.picker = nil
			m.updateColumns()
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		for i := range m.lists {
			m.lists[i].SetSize(30, 15)
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "right", "tab", "shift+tab":
			m.focusSide = (m.focusSide + 1) % 2
			return m, nil

		case "enter":
			if m.lists[m.focusSide].SelectedItem() != nil {
				m.isPicking = true
				m.picker = iconpicker.NewIconPicker(m.currentIconType)
			}
			return m, nil

		case "m":
			switch m.currentIconType {
			case config.IconTypeEmoji:
				m.currentIconType = config.IconTypeNerdFonts
			case config.IconTypeNerdFonts:
				m.currentIconType = config.IconTypeASCII
			case config.IconTypeASCII:
				m.currentIconType = config.IconTypeEmoji
			}
			m.updateColumns()
			return m, nil

		case "h":
			item := m.lists[m.focusSide].SelectedItem()
			if item == nil {
				return m, nil
			}
			selected := item.(libConfigItem)

			cfg := config.Get()
			id, srv, ok := cfg.GetActiveServer()
			if ok {
				if srv.Libraries.Settings == nil {
					srv.Libraries.Settings = make(map[string]config.LibraryOptions)
				}
				opts := srv.Libraries.Settings[selected.id]

				if m.focusSide == 0 { // Hidden -> Active
					opts.Hidden = false
				} else { // Active -> Hidden
					opts.Hidden = true
				}

				srv.Libraries.Settings[selected.id] = opts
				cfg.Servers[id] = srv
				_ = cfg.Save()
				m.updateColumns()
			}
			return m, nil

		case "j", "k":
			if m.focusSide != 1 { // Only reorder in Active column
				break
			}
			idx := m.lists[1].Index()
			items := m.lists[1].Items()
			if len(items) < 2 {
				break
			}

			newIdx := idx
			if msg.String() == "j" && idx < len(items)-1 {
				newIdx = idx + 1
			} else if msg.String() == "k" && idx > 0 {
				newIdx = idx - 1
			}

			if newIdx != idx {
				cfg := config.Get()
				id, srv, ok := cfg.GetActiveServer()
				if ok {
					order := srv.Libraries.Order
					id1 := items[idx].(libConfigItem).id
					id2 := items[newIdx].(libConfigItem).id

					idx1 := -1
					idx2 := -1
					for i, oid := range order {
						if oid == id1 {
							idx1 = i
						}
						if oid == id2 {
							idx2 = i
						}
					}

					// If not in order list yet, add it
					if idx1 == -1 {
						order = append(order, id1)
						idx1 = len(order) - 1
					}
					if idx2 == -1 {
						order = append(order, id2)
						idx2 = len(order) - 1
					}

					order[idx1], order[idx2] = order[idx2], order[idx1]
					srv.Libraries.Order = order
					cfg.Servers[id] = srv
					_ = cfg.Save()
					m.updateColumns()
					m.lists[1].Select(newIdx)
				}
			}
			return m, nil

		case "esc":
			return nil, func() tea.Msg { return ConfigFinishedMsg{IconType: m.currentIconType} }
		}
	}

	m.lists[m.focusSide], cmd = m.lists[m.focusSide].Update(msg)
	return m, cmd
}

func (m *ConfigOverlayModel) View() string {
	theme := ui.CurrentTheme()
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(ui.Accent(theme)).
		Padding(1, 2).
		Background(lipgloss.Color("#111111"))

	if m.isPicking && m.picker != nil {
		return overlayStyle.Render(m.picker.View())
	}

	listStyle := lipgloss.NewStyle().Padding(1)
	activeListStyle := listStyle.Copy().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(ui.Accent(theme))

	var views []string
	for i := range m.lists {
		lView := m.lists[i].View()
		if m.focusSide == i {
			views = append(views, activeListStyle.Render(lView))
		} else {
			views = append(views, listStyle.Render(lView))
		}
	}

	content := lipgloss.JoinHorizontal(lipgloss.Top, views...)

	modeLabel := lipgloss.NewStyle().Foreground(theme.BrightBlue()).Bold(true).Render(" Mode: " + string(m.currentIconType))

	return overlayStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		modeLabel,
		content,
		"\n [arrows/tab] switch | [enter] icon | [m] type | [h] hide | [j/k] order | [esc] finish",
	))
}
