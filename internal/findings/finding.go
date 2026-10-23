// Package findings reads a nitpicker findings store (docs/audit/findings/):
// the open finding files and the resolved ledger. It imports the standard
// library only, and it never writes: everything it reads comes through an
// fs.FS.
package findings

// Finding is one finding from the store: an open file under
// <auditor>/open/, or a record from resolved.jsonl. Every string field has
// been through Sanitize.
type Finding struct {
	ID       string
	Auditor  string
	Severity string
	Category string
	Area     string
	Status   string
	Found    string
	Resolved string
	Location string
	Title    string
	Body     string
	// Path is the store-relative file the finding was read from.
	Path string

	// haystack is the lowercase text Filter searches.
	haystack string
}
