package presenters

// Presenter defines how data should be formatted for output
type Presenter interface {
	// Title returns the title of the output (for table/text views)
	Title() string

	// Headers returns the column headers for table/csv views
	Headers() []string

	// Rows returns the stringified data for table/csv/text views
	Rows() [][]string

	// Raw returns the underlying data structure for JSON/YAML output
	Raw() interface{}

	// SortableColumns returns a list of columns that can be sorted
	SortableColumns() []string

	// SortBy sorts the data by the given column. Returns true if sorted, false if column is invalid.
	SortBy(column string) bool

	// DefaultSort returns the default sort column
	DefaultSort() string
}
