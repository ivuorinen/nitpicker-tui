package findings

import (
	"maps"
	"strings"
)

// Query selects findings: every whitespace-separated term of Text must occur
// in the id, title, area or body (case-insensitive), and every facet in Sel
// must equal its selected value. A facet absent from Sel matches anything;
// one selected as "" matches only findings with no value for it.
type Query struct {
	Text string
	Sel  map[Facet]string
}

func (q Query) match(x Finding, terms []string) bool {
	for f, v := range q.Sel {
		if f.Of(x) != v {
			return false
		}
	}
	for _, t := range terms {
		if !strings.Contains(x.haystack, t) {
			return false
		}
	}

	return true
}

func (q Query) terms() []string {
	return strings.Fields(strings.ToLower(q.Text))
}

// Filter returns the indices of the findings in fs that match q, in order.
func Filter(fs []Finding, q Query) []int {
	terms := q.terms()
	out := []int{}
	for i, x := range fs {
		if q.match(x, terms) {
			out = append(out, i)
		}
	}

	return out
}

// Counts counts, for each value of facet f, the findings that match q's text
// and every other facet: what selecting that value would show. total is the
// count with f unselected.
func Counts(fs []Finding, q Query, f Facet) (total int, per map[string]int) {
	others := Query{Text: q.Text, Sel: maps.Clone(q.Sel)}
	delete(others.Sel, f)
	terms := others.terms()
	per = map[string]int{}
	for _, x := range fs {
		if others.match(x, terms) {
			total++
			per[f.Of(x)]++
		}
	}

	return total, per
}
