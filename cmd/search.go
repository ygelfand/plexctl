package cmd

import (
	"fmt"
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
		entries, indexedAt := idx.Snapshot()

		lastIndexed := "Never"
		if !indexedAt.IsZero() {
			lastIndexed = indexedAt.Format("2006-01-02 15:04:05")
		}

		data := map[string]any{
			"last_indexed":  lastIndexed,
			"total_entries": len(entries),
		}

		return ui.OutputData{
			Title:   "SEARCH INDEX STATUS",
			Headers: []string{"PROPERTY", "VALUE"},
			Rows: [][]string{
				{"Last Indexed", lastIndexed},
				{"Total Entries", fmt.Sprintf("%d", len(entries))},
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
		finished := make(chan error, 1)
		idx := search.GetIndex()
		theme := ui.CurrentTheme()

		fmt.Println(ui.TitleStyle(theme).Render("Starting Reindex..."))

		go func() {
			defer close(progress)
			finished <- idx.Reindex(cmd.Context(), progress)
		}()

		for p := range progress {
			fmt.Printf("\r\033[K[%d/%d] %s", p.Current, p.Total, p.Message)
		}
		fmt.Println()
		if err := <-finished; err != nil {
			return err
		}
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
		entries, _ := idx.Snapshot()

		if len(entries) == 0 {
			return fmt.Errorf("index is empty. Please run 'plexctl search reindex' first")
		}

		matches := fuzzy.FindFrom(query, entrySource(entries))

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
			e := entries[match.Index]
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

func (s entrySource) String(i int) string { return s[i].SearchableText() }
func (s entrySource) Len() int            { return len(s) }

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.AddCommand(searchStatusCmd)
	searchCmd.AddCommand(searchReindexCmd)
	searchCmd.AddCommand(searchFindCmd)
}
