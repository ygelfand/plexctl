package commands

// PlexCtlOptions holds common command-line flags and options
type PlexCtlOptions struct {
	OutputFormat string
	Verbosity    int
	Sort         string
	Count        int
	Page         int
	All          bool
}
