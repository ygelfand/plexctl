package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/search"
	"github.com/ygelfand/plexctl/internal/ui"
)

var searchCmd = &cobra.Command{
	Use:     "search",
	Short:   "Manage and use the library search index",
	GroupID: "media",
}

var searchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show search index status and library statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		idx := search.GetIndex()
		theme := ui.CurrentTheme()

		titleStyle := ui.TitleStyle(theme)
		labelStyle := ui.LabelStyle(theme)
		valueStyle := ui.ValueStyle(theme)

		fmt.Println(titleStyle.Render("SEARCH INDEX STATUS"))
		lastIndexed := "Never"
		if !idx.LastIndexed.IsZero() {
			lastIndexed = idx.LastIndexed.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%s %s\n", labelStyle.Render("Last Indexed:"), valueStyle.Render(lastIndexed))
		fmt.Printf("%s %s\n", labelStyle.Render("Total Entries:"), valueStyle.Render(fmt.Sprintf("%d", len(idx.Entries))))

		return nil
	},
}

var searchReindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Rebuild the local search index",
	RunE: func(cmd *cobra.Command, args []string) error {
		progress := make(chan search.IndexProgress, 100)
		idx := search.GetIndex()
		theme := ui.CurrentTheme()

		fmt.Println(ui.TitleStyle(theme).Render("Starting Reindex..."))

		go func() {
			err := idx.Reindex(context.Background(), progress)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			}
			progress <- search.IndexProgress{Message: "DONE"}
		}()

		for p := range progress {
			if p.Message == "DONE" {
				break
			}
			fmt.Printf("\r\033[K[%d/%d] %s", p.Current, p.Total, p.Message)
		}
		fmt.Println()
		fmt.Println(ui.SuccessStyle(theme).Render("Indexing complete!"))
		return nil
	},
}

var searchFindCmd = &cobra.Command{
	Use:   "find [query]",
	Short: "Search for items in the index",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		idx := search.GetIndex()
		theme := ui.CurrentTheme()

		if len(idx.Entries) == 0 {
			return fmt.Errorf("index is empty. Please run 'plexctl search reindex' first")
		}

		matches := fuzzy.FindFrom(query, entrySource(idx.Entries))

		if len(matches) == 0 {
			fmt.Println("No matches found.")
			return nil
		}

		fmt.Println(ui.TitleStyle(theme).Render(fmt.Sprintf("Results for: %s", query)))

		headers := []string{"TITLE", "TYPE", "LIBRARY", "KEY"}
		var rows [][]string

		for i, match := range matches {
			if i >= 20 {
				break
			}
			e := idx.Entries[match.Index]
			rows = append(rows, []string{
				e.Title,
				strings.ToUpper(e.Type),
				e.Library,
				e.RatingKey,
			})
		}

		ui.OutputData{
			Headers: headers,
			Rows:    rows,
			Raw:     rows, // Use rows as raw for now
		}.Print()

		return nil
	},
}

type entrySource []search.IndexEntry

func (s entrySource) String(i int) string { return s[i].Title }
func (s entrySource) Len() int            { return len(s) }

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.AddCommand(searchStatusCmd)
	searchCmd.AddCommand(searchReindexCmd)
	searchCmd.AddCommand(searchFindCmd)
}
