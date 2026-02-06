package presenters

// SimplePresenter is a generic presenter for simple table/raw data
type SimplePresenter struct {
	T       string
	H       []string
	R       [][]string
	RawData interface{}
}

func (p SimplePresenter) Title() string             { return p.T }
func (p SimplePresenter) Headers() []string         { return p.H }
func (p SimplePresenter) Rows() [][]string          { return p.R }
func (p SimplePresenter) Raw() interface{}          { return p.RawData }
func (p SimplePresenter) SortableColumns() []string { return []string{} }
func (p SimplePresenter) SortBy(column string) bool { return false }
func (p SimplePresenter) DefaultSort() string       { return "" }
