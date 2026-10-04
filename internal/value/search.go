package value

// Search request and page records preserve native cursor fingerprint shapes.
type SearchItem struct{ LUID, Type, Name, ProjectPath, Owner, ModifiedAt string }
type SearchPage struct {
	UnresolvedMoreAvailable bool
	Total                   int
	Items                   []SearchItem
	NextCursor              string
	Warnings                []string
	TableauRequestID        string
	Source                  string
	MoreAvailable           bool
}
type SearchRequest struct {
	Types                             []string
	Terms, ProjectPath, Owner, Cursor string
	Limit                             int
}
