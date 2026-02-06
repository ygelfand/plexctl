package ui

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/TylerBrock/colorjson"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/olekukonko/tablewriter"
	"github.com/ygelfand/plexctl/internal/config"
	"gopkg.in/yaml.v3"
)

var PlexOrange = lipgloss.Color("#e5a00d")

type PlexctlTint struct{}

func (t *PlexctlTint) DisplayName() string { return "Plexctl" }
func (t *PlexctlTint) ID() string          { return "plexctl" }
func (t *PlexctlTint) About() string       { return "Plexctl default theme" }

func (t *PlexctlTint) Fg() lipgloss.TerminalColor          { return lipgloss.Color("#cccccc") }
func (t *PlexctlTint) Bg() lipgloss.TerminalColor          { return lipgloss.Color("#1a1a1a") }
func (t *PlexctlTint) SelectionBg() lipgloss.TerminalColor { return lipgloss.Color("#333333") }
func (t *PlexctlTint) Cursor() lipgloss.TerminalColor      { return PlexOrange }

func (t *PlexctlTint) BrightBlack() lipgloss.TerminalColor  { return lipgloss.Color("#4d4d4d") }
func (t *PlexctlTint) BrightBlue() lipgloss.TerminalColor   { return lipgloss.Color("#5bc0de") }
func (t *PlexctlTint) BrightCyan() lipgloss.TerminalColor   { return lipgloss.Color("#5bc0de") }
func (t *PlexctlTint) BrightGreen() lipgloss.TerminalColor  { return lipgloss.Color("#5cb85c") }
func (t *PlexctlTint) BrightPurple() lipgloss.TerminalColor { return lipgloss.Color("#d9534f") }
func (t *PlexctlTint) BrightRed() lipgloss.TerminalColor    { return lipgloss.Color("#d9534f") }
func (t *PlexctlTint) BrightWhite() lipgloss.TerminalColor  { return lipgloss.Color("#ffffff") }
func (t *PlexctlTint) BrightYellow() lipgloss.TerminalColor { return lipgloss.Color("#f0ad4e") }

func (t *PlexctlTint) Black() lipgloss.TerminalColor  { return lipgloss.Color("#000000") }
func (t *PlexctlTint) Blue() lipgloss.TerminalColor   { return lipgloss.Color("#337ab7") }
func (t *PlexctlTint) Cyan() lipgloss.TerminalColor   { return lipgloss.Color("#5bc0de") }
func (t *PlexctlTint) Green() lipgloss.TerminalColor  { return lipgloss.Color("#5cb85c") }
func (t *PlexctlTint) Purple() lipgloss.TerminalColor { return lipgloss.Color("#d9534f") }
func (t *PlexctlTint) Red() lipgloss.TerminalColor    { return lipgloss.Color("#d9534f") }
func (t *PlexctlTint) White() lipgloss.TerminalColor  { return lipgloss.Color("#cccccc") }
func (t *PlexctlTint) Yellow() lipgloss.TerminalColor { return lipgloss.Color("#f0ad4e") }

var PlexctlTheme = &PlexctlTint{}

// CurrentTheme returns the theme currently configured in config.Get()
func CurrentTheme() tint.Tint {
	cfg := config.Get()
	tints := append([]tint.Tint{PlexctlTheme}, tint.DefaultTints()...)
	for _, t := range tints {
		if t.ID() == cfg.Theme {
			return t
		}
	}
	return PlexctlTheme
}

// Accent returns the primary accent color for the theme (Plex Orange for our theme, or Cyan/Blue for others)
func Accent(t tint.Tint) lipgloss.TerminalColor {
	if t.ID() == "plexctl" {
		return PlexOrange
	}
	return t.BrightCyan()
}

func AccentStyle(t tint.Tint) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(Accent(t))
}

func TitleStyle(t tint.Tint) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(Accent(t)).
		MarginBottom(1)
}

func LabelStyle(t tint.Tint) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(t.BrightWhite()).
		Width(20)
}

func ValueStyle(t tint.Tint) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(t.White())
}

func TableColumnHeaderStyle(t tint.Tint) lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(Accent(t)).
		Underline(true)
}

func ErrorStyle(t tint.Tint) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(t.BrightRed()).
		Bold(true)
}

func SuccessStyle(t tint.Tint) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(t.BrightGreen()).
		Bold(true)
}

// RenderError prints a styled error message
func RenderError(err error) {
	fmt.Fprintf(os.Stderr, "%s %v\n", ErrorStyle(CurrentTheme()).Render("Error:"), err)
}

// RenderSuccess prints a styled success message
func RenderSuccess(msg string) {
	fmt.Println(SuccessStyle(CurrentTheme()).Render(msg))
}

// OutputData represents data that can be printed in multiple formats
type OutputData struct {
	Title   string
	Headers []string
	Rows    [][]string
	Raw     interface{} // Used for JSON/YAML
}

// Print handles the output based on the configured format
func (d OutputData) Print() error {
	cfg := config.Get()
	// Robustly handle potentially quoted format strings from config
	format := strings.Trim(strings.ToLower(cfg.OutputFormat), "\"")

	switch format {
	case "json":
		return d.printJSON()
	case "json-pretty":
		return d.printJSONPretty()
	case "yaml":
		return d.printYAML()
	case "csv":
		return d.printCSV()
	case "txt", "text":
		return d.printText()
	case "table":
		fallthrough
	default:
		return d.printTable()
	}
}

func (d OutputData) printJSONPretty() error {
	rawJSON, err := json.Marshal(d.Raw)
	if err != nil {
		return err
	}
	var obj any
	if err := json.Unmarshal(rawJSON, &obj); err != nil {
		return err
	}

	f := colorjson.NewFormatter()
	f.Indent = 2
	b, err := f.Marshal(obj)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func (d OutputData) printJSON() error {
	b, err := json.Marshal(d.Raw)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func (d OutputData) printYAML() error {
	b, err := yaml.Marshal(d.Raw)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func (d OutputData) printCSV() error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write(d.Headers); err != nil {
		return err
	}
	if err := w.WriteAll(d.Rows); err != nil {
		return err
	}
	w.Flush()
	return nil
}

func (d OutputData) printText() error {
	theme := CurrentTheme()
	if d.Title != "" {
		fmt.Println(TitleStyle(theme).Render(d.Title))
	}
	for _, row := range d.Rows {
		for i, val := range row {
			if i < len(d.Headers) {
				fmt.Printf("%s %s\n", LabelStyle(theme).Render(d.Headers[i]+":"), ValueStyle(theme).Render(val))
			}
		}
		fmt.Println()
	}
	return nil
}

func (d OutputData) printTable() error {
	if d.Title != "" {
		fmt.Println(TitleStyle(CurrentTheme()).Render(d.Title))
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.Header(d.Headers)
	table.Bulk(d.Rows)
	return table.Render()
}

// RenderSummary renders a list of key-value pairs
func RenderSummary(title string, items []struct{ Label, Value string }) {
	theme := CurrentTheme()
	if title != "" {
		fmt.Println(TitleStyle(theme).Render(title))
	}

	var b strings.Builder
	for _, item := range items {
		b.WriteString(fmt.Sprintf("%s %s\n", LabelStyle(theme).Render(item.Label+":"), ValueStyle(theme).Render(item.Value)))
	}
	fmt.Println(b.String())
}

func Ptr[T any](v T) *T {
	return &v
}
