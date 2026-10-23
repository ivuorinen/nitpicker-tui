package findings

import (
	"fmt"
	"strings"
	"testing"
)

// benchStore is 5000 findings with 2 KiB bodies, spread over 35 lenses: past
// the largest real store (925 ledger records, 144 open files) by a margin.
func benchStore() []Finding {
	severities := severityOrder
	statuses := []string{StatusOpen, statusFixed, statusFixed, statusFixed, statusInvalid}
	categories := []string{catCorrectness, catSecurity, catDocs, "tests", ""}
	filler := strings.Repeat("the quick brown fox jumps over the lazy dog ", 47)
	var s Store
	for i := range 5000 {
		body := filler
		if i%7 == 0 {
			body += "race condition"
		}
		s.Findings = append(s.Findings, clean(Finding{
			ID:       fmt.Sprintf("lens%02d-%08x", i%35, i),
			Auditor:  fmt.Sprintf("lens%02d", i%35),
			Severity: severities[i%len(severities)],
			Category: categories[i%len(categories)],
			Status:   statuses[i%len(statuses)],
			Title:    fmt.Sprintf("Finding number %d", i),
			Body:     body,
		}))
	}

	return s.Findings
}

func benchQuery() Query {
	return Query{Text: "race condition", Sel: map[Facet]string{
		State: statusFixed, Lens: "lens07", Severity: severityHigh, Category: catSecurity,
	}}
}

func BenchmarkFilter(b *testing.B) {
	all, q := benchStore(), benchQuery()
	for b.Loop() {
		Filter(all, q)
	}
}

func BenchmarkCounts(b *testing.B) {
	all, q := benchStore(), benchQuery()
	for b.Loop() {
		Counts(all, q, Lens)
	}
}
