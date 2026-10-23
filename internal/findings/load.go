package findings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// LedgerName is the resolved-findings ledger at the store root.
const LedgerName = "resolved.jsonl"

// Store is everything Load read: the findings, and one warning for each file
// or ledger line it could not read cleanly.
type Store struct {
	Findings []Finding
	Warnings []string
}

// Load reads a findings store: <auditor>/open/*.md and resolved.jsonl at the
// root of fsys. It keeps no state, so reloading the store is another call.
//
// Only regular files and directories are read; a symlink is skipped with a
// warning, as findings.py refuses them. Any other problem with one file or
// one ledger line also becomes a warning, never a silently dropped finding.
// Load fails only when the store root cannot be listed.
func Load(fsys fs.FS) (Store, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return Store{}, fmt.Errorf("list findings store: %w", err)
	}
	var s Store
	for _, e := range entries {
		switch {
		case e.Type()&fs.ModeSymlink != 0:
			s.warn("%s: symlink; skipped", e.Name())
		case e.IsDir():
			s.loadOpen(fsys, e.Name())
		case e.Name() == LedgerName:
			s.loadLedger(fsys, e)
		}
	}

	return s, nil
}

func (s *Store) loadOpen(fsys fs.FS, auditor string) {
	dir := path.Join(auditor, StatusOpen)
	// ReadDir follows a symlink that stays inside an os.Root; findings.py
	// refuses it, and so does this.
	if info, err := fs.Lstat(fsys, dir); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		s.warn("%s: symlink; skipped", dir)

		return
	}
	entries, err := fs.ReadDir(fsys, dir)
	if errors.Is(err, fs.ErrNotExist) {
		// Every finding of this auditor is resolved.
		return
	}
	if err != nil {
		s.warn("%s: %v", dir, err)

		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		p := path.Join(dir, name)
		if !e.Type().IsRegular() {
			s.warn("%s: not a regular file; skipped", p)

			continue
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			s.warn("%s: %v", p, err)

			continue
		}
		f, ok := openFinding(string(data), p, auditor)
		if !ok {
			s.warn("%s: no frontmatter; listed under its file name", p)
		}
		s.Findings = append(s.Findings, f)
	}
}

// openFinding builds the finding in the open file at p (under auditor/open/).
// ok is false when the file has no frontmatter. An open file is open whatever
// its frontmatter says, as in findings.py list, and findings.py falls back to
// the file name only when the id key is missing: an empty id stays empty.
func openFinding(text, p, auditor string) (f Finding, ok bool) {
	fm, title, body, ok := parseFinding(text)
	id, hasID := fm["id"]
	if !hasID {
		id = strings.TrimSuffix(path.Base(p), ".md")
	}
	auditorName := fm["auditor"]
	if auditorName == "" {
		auditorName = auditor
	}

	return clean(Finding{
		ID:       id,
		Auditor:  auditorName,
		Severity: fm["severity"],
		Category: fm["category"],
		Area:     fm["area"],
		Status:   StatusOpen,
		Found:    fm["found"],
		Location: fm["location"],
		Title:    title,
		Body:     body,
		Path:     p,
	}), ok
}

// loadLedger reads the whole ledger and splits it like findings.py does
// (str.splitlines); a bufio.Scanner would stop at the first line over 64 KiB.
// A non-regular entry (a FIFO would block ReadFile forever) is skipped. As in
// findings.py's listing, the last record for an id is the one shown.
func (s *Store) loadLedger(fsys fs.FS, e fs.DirEntry) {
	if !e.Type().IsRegular() {
		s.warn("%s: not a regular file; skipped", LedgerName)

		return
	}
	data, err := fs.ReadFile(fsys, LedgerName)
	if err != nil {
		s.warn("%s: %v", LedgerName, err)

		return
	}
	seen := map[string]int{}
	for i, line := range splitLines(string(data)) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r, err := decodeRecord(line)
		if err != nil {
			s.warn("%s:%d: %v", LedgerName, i+1, err)

			continue
		}
		f := clean(r.finding())
		if j, dup := seen[r.id]; dup {
			s.Findings[j] = f

			continue
		}
		seen[r.id] = len(s.Findings)
		s.Findings = append(s.Findings, f)
	}
}

// ledgerRecord is one resolved.jsonl line, as findings.py writes it.
type ledgerRecord struct {
	id, auditor, severity, category, area, status, found, resolved string
	title, body, location                                          string
}

// decodeRecord reads one ledger line by its exact keys: encoding/json would
// match "ID" to "id", which findings.py does not. A line that is not a JSON
// object (null, a number, an array) is an error, as it is in findings.py.
func decodeRecord(line string) (ledgerRecord, error) {
	if !strings.HasPrefix(line, "{") {
		return ledgerRecord{}, errors.New("not a JSON object")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return ledgerRecord{}, err
	}
	var r ledgerRecord
	for key, dst := range map[string]*string{
		"id": &r.id, "auditor": &r.auditor, "severity": &r.severity, "category": &r.category,
		"area": &r.area, "status": &r.status, "found": &r.found, "resolved": &r.resolved,
		"title": &r.title, "body": &r.body, "location": &r.location,
	} {
		if err := stringField(raw, key, dst); err != nil {
			return ledgerRecord{}, err
		}
	}
	// findings.py reads rec.get("location") or extra.location.
	if r.location == "" {
		if extraRaw, ok := raw["extra"]; ok {
			var extra map[string]json.RawMessage
			if err := json.Unmarshal(extraRaw, &extra); err != nil {
				return ledgerRecord{}, fmt.Errorf("extra: %w", err)
			}
			if err := stringField(extra, "location", &r.location); err != nil {
				return ledgerRecord{}, fmt.Errorf("extra: %w", err)
			}
		}
	}

	return r, nil
}

// stringField decodes raw[key] into dst when the key is present.
func stringField(raw map[string]json.RawMessage, key string, dst *string) error {
	v, ok := raw[key]
	if !ok {
		return nil
	}
	if err := json.Unmarshal(v, dst); err != nil {
		return fmt.Errorf("field %q: %w", key, err)
	}

	return nil
}

func (r ledgerRecord) finding() Finding {
	return Finding{
		ID:       r.id,
		Auditor:  r.auditor,
		Severity: r.severity,
		Category: r.category,
		Area:     r.area,
		Status:   r.status,
		Found:    r.found,
		Resolved: r.resolved,
		Location: r.location,
		Title:    r.title,
		Body:     r.body,
		Path:     LedgerName,
	}
}

// clean sanitizes every field and builds the search text: the point where
// store content becomes displayable text. Every field except Body is drawn on
// one line, so line breaks and tabs in them become spaces; a newline in a
// title would otherwise draw a row of its own in the list. Rendering markdown
// decodes character references afterwards, so the renderer filters its
// output as well (ui.keepStyling).
func clean(f Finding) Finding {
	f.Body = Sanitize(f.Body)
	for _, field := range []*string{
		&f.ID, &f.Auditor, &f.Severity, &f.Category, &f.Area, &f.Status,
		&f.Found, &f.Resolved, &f.Location, &f.Title, &f.Path,
	} {
		*field = oneLine(Sanitize(*field))
	}
	f.haystack = strings.ToLower(strings.Join([]string{f.ID, f.Title, f.Area, f.Body}, "\n"))

	return f
}

// oneLine replaces the line breaks and tabs Sanitize keeps with spaces.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}

		return r
	}, s)
}

// warn records a warning. File names come from the repository too, so the
// message is sanitized like any other store text.
func (s *Store) warn(format string, args ...any) {
	s.Warnings = append(s.Warnings, oneLine(Sanitize(fmt.Sprintf(format, args...))))
}
