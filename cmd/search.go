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

		lastIndexed := "Never"
		if !idx.LastIndexed.IsZero() {
			lastIndexed = idx.LastIndexed.Format("2006-01-02 15:04:05")
		}

		data := map[string]interface{}{
			"last_indexed":  lastIndexed,
			"total_entries": len(idx.Entries),
		}

		return ui.OutputData{
			Title:   "SEARCH INDEX STATUS",
			Headers: []string{"PROPERTY", "VALUE"},
			Rows: [][]string{
				{"Last Indexed", lastIndexed},
				{"Total Entries", fmt.Sprintf("%d", len(idx.Entries))},
			},
			Raw: data,
		}.Print()
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

		if len(idx.Entries) == 0 {
			return fmt.Errorf("index is empty. Please run 'plexctl search reindex' first")
		}

		matches := fuzzy.FindFrom(query, entrySource(idx.Entries))

		if len(matches) == 0 {
			fmt.Println("No matches found.")
			return nil
		}

		headers := []string{"ID", "TITLE", "TYPE", "LIBRARY"}
		var rows [][]string
		var rawResults []search.IndexEntry

		for i, match := range matches {
			if i >= 20 {
				break
			}
			e := idx.Entries[match.Index]
			rows = append(rows, []string{
				e.RatingKey,
				e.Title,
				strings.ToUpper(e.Type),
				e.Library,
			})
			rawResults = append(rawResults, e)
		}

		return ui.OutputData{
			Title:   fmt.Sprintf("Results for: %s", query),
			Headers: headers,
			Rows:    rows,
			Raw:     rawResults,
		}.Print()
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
