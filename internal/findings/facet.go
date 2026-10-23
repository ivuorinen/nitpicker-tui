package findings

import (
	"cmp"
	"slices"
	"strings"
)

// Facet is one field findings can be filtered by. Every facet's values come
// from the loaded findings, so one picker and one Counts serve all four.
type Facet int

// The facets, in display order.
const (
	State Facet = iota
	Lens
	Severity
	Category
)

// Facets lists every facet in display order.
var Facets = []Facet{State, Lens, Severity, Category}

// StatusOpen is the state of a finding that is not resolved yet.
const StatusOpen = "open"

// The rest of the vocabularies findings.py writes (STATUSES, SEVERITIES).
const (
	statusFixed   = "fixed"
	statusInvalid = "invalid"

	severityCritical = "critical"
	severityHigh     = "high"
	severityMedium   = "medium"
	severityLow      = "low"
	severityAdvisory = "advisory"
)

// Known values rank in this order; anything else sorts after them, A-Z, and
// the empty value sorts last.
var (
	statusOrder   = []string{StatusOpen, statusFixed, statusInvalid}
	severityOrder = []string{severityCritical, severityHigh, severityMedium, severityLow, severityAdvisory}
)

func (f Facet) String() string {
	switch f {
	case State:
		return "state"
	case Lens:
		return "lens"
	case Severity:
		return "severity"
	case Category:
		return "category"
	}

	return ""
}

// Of returns the finding's value for this facet. The lens is the auditor: the
// command that filed the finding.
func (f Facet) Of(x Finding) string {
	switch f {
	case State:
		return x.Status
	case Lens:
		return x.Auditor
	case Severity:
		return x.Severity
	case Category:
		return x.Category
	}

	return ""
}

func (f Facet) order() []string {
	switch f {
	case State:
		return statusOrder
	case Severity:
		return severityOrder
	case Lens, Category:
	}

	return nil
}

// compare orders two values of the facet: known values by rank, then the
// rest A-Z, then the empty value.
func (f Facet) compare(a, b string) int {
	if (a == "") != (b == "") {
		if a == "" {
			return 1
		}

		return -1
	}
	order := f.order()
	if c := cmp.Compare(rank(order, a), rank(order, b)); c != 0 {
		return c
	}

	return strings.Compare(a, b)
}

func rank(order []string, v string) int {
	if i := slices.Index(order, v); i >= 0 {
		return i
	}

	return len(order)
}

// Values returns the distinct values of the facet across fs, in facet order.
func Values(fs []Finding, f Facet) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range fs {
		if v := f.Of(x); !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	slices.SortFunc(out, f.compare)

	return out
}

// Sort orders findings for the list: by state, then severity, then newest
// found date first, then id.
func Sort(fs []Finding) {
	slices.SortStableFunc(fs, func(a, b Finding) int {
		return cmp.Or(
			State.compare(a.Status, b.Status),
			Severity.compare(a.Severity, b.Severity),
			strings.Compare(b.Found, a.Found),
			strings.Compare(a.ID, b.ID),
		)
	})
}
