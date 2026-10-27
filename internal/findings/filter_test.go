package findings

import (
	"maps"
	"slices"
	"testing"
)

// build runs findings through clean, the path Load uses, so they carry the
// same search text a loaded store does.
func build(fs ...Finding) []Finding {
	out := make([]Finding, 0, len(fs))
	for _, f := range fs {
		out = append(out, clean(f))
	}

	return out
}

func fixture() []Finding {
	return build(
		Finding{
			ID: idA1, Auditor: lensAudit, Severity: severityHigh, Category: catCorrectness, Status: StatusOpen,
			Title: "Race condition in cache", Body: "lock missing",
		},
		Finding{
			ID: idA2, Auditor: lensAudit, Severity: severityLow, Category: catDocs, Status: StatusOpen,
			Title: "Typo", Area: "README.md",
		},
		Finding{
			ID: idS1, Auditor: lensSecurity, Severity: severityCritical, Category: catSecurity, Status: StatusOpen,
			Title: "Token in LOG", Body: "race",
		},
		Finding{
			ID: idS2, Auditor: lensSecurity, Severity: severityMedium, Status: statusFixed,
			Title: "Old bug", Body: "race lock",
		},
		Finding{
			ID: idC1, Auditor: lensCI, Severity: severityAdvisory, Category: "conventions", Status: statusInvalid,
			Title: "Unpinned action",
		},
		Finding{ID: idC2, Auditor: lensCI, Status: statusFixed, Title: "Something", Body: "x"},
	)
}

func ids(all []Finding, idx []int) []string {
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		out = append(out, all[i].ID)
	}

	return out
}

func TestFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"empty query", Query{}, []string{idA1, idA2, idS1, idS2, idC1, idC2}},
		{"state", Query{Sel: map[Facet]string{State: StatusOpen}}, []string{idA1, idA2, idS1}},
		{"lens", Query{Sel: map[Facet]string{Lens: lensSecurity}}, []string{idS1, idS2}},
		{"severity none", Query{Sel: map[Facet]string{Severity: ""}}, []string{idC2}},
		{"category none", Query{Sel: map[Facet]string{Category: ""}}, []string{idS2, idC2}},
		{"two terms are ANDed", Query{Text: "race lock"}, []string{idA1, idS2}},
		{"case-insensitive", Query{Text: "RACE"}, []string{idA1, idS1, idS2}},
		{"matches id", Query{Text: idC1}, []string{idC1}},
		{"matches area", Query{Text: "readme.md"}, []string{idA2}},
		{
			"all four facets and text",
			Query{Text: "token", Sel: map[Facet]string{
				State: StatusOpen, Lens: lensSecurity, Severity: severityCritical, Category: catSecurity,
			}},
			[]string{idS1},
		},
		{"no match", Query{Sel: map[Facet]string{State: statusFixed, Lens: lensAudit}}, []string{}},
	}

	all := fixture()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ids(all, Filter(all, tt.q)); !slices.Equal(got, tt.want) {
				t.Errorf("Filter = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValues(t *testing.T) {
	t.Parallel()

	// Extra severity and state values only; the auditor repeats a known lens.
	extra := build(
		Finding{Auditor: lensAudit, Severity: severityUnknown, Status: "wontfix"},
		Finding{Auditor: lensAudit, Severity: "bogus", Status: ""},
	)
	all := append(fixture(), extra...)

	tests := []struct {
		facet Facet
		want  []string
	}{
		// "ci" appears only in resolved findings.
		{Lens, []string{lensAudit, lensCI, lensSecurity}},
		{State, []string{StatusOpen, statusFixed, statusInvalid, "wontfix", ""}},
		{Severity, []string{
			severityCritical, severityHigh, severityMedium, severityLow, severityAdvisory,
			"bogus", severityUnknown, "",
		}},
		{Category, []string{"conventions", catCorrectness, catDocs, catSecurity, ""}},
	}
	for _, tt := range tests {
		t.Run(tt.facet.String(), func(t *testing.T) {
			t.Parallel()

			if got := Values(all, tt.facet); !slices.Equal(got, tt.want) {
				t.Errorf("Values(%s) = %q, want %q", tt.facet, got, tt.want)
			}
		})
	}
}

func TestCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		q         Query
		facet     Facet
		wantTotal int
		wantPer   map[string]int
	}{
		{
			"ignores the counted facet's own selection",
			Query{Sel: map[Facet]string{State: StatusOpen, Lens: lensAudit}}, Lens,
			3, map[string]int{lensAudit: 2, lensSecurity: 1},
		},
		{
			"other facets narrow the count",
			Query{Sel: map[Facet]string{Lens: lensSecurity}}, State,
			2, map[string]int{StatusOpen: 1, statusFixed: 1},
		},
		{
			"search text narrows the count",
			Query{Text: "race"}, Severity,
			3, map[string]int{severityHigh: 1, severityCritical: 1, severityMedium: 1},
		},
	}

	all := fixture()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			total, per := Counts(all, tt.q, tt.facet)
			if total != tt.wantTotal || !maps.Equal(per, tt.wantPer) {
				t.Errorf("Counts = %d %v, want %d %v", total, per, tt.wantTotal, tt.wantPer)
			}
		})
	}
}

func TestCountsLeavesQueryUnchanged(t *testing.T) {
	t.Parallel()

	q := Query{Sel: map[Facet]string{State: StatusOpen, Lens: lensAudit}}
	Counts(fixture(), q, Lens)
	if q.Sel[Lens] != lensAudit {
		t.Errorf("Counts removed the caller's lens selection: %v", q.Sel)
	}
}

func TestSort(t *testing.T) {
	t.Parallel()

	fs := []Finding{
		{ID: "i-crit", Status: statusInvalid, Severity: severityCritical},
		{ID: "o-none", Status: StatusOpen},
		{ID: "o-b", Status: StatusOpen, Severity: severityLow, Found: "2026-03-01"},
		{ID: "f-high", Status: statusFixed, Severity: severityHigh},
		{ID: "o-adv", Status: StatusOpen, Severity: severityAdvisory},
		{ID: "o-crit", Status: StatusOpen, Severity: severityCritical, Found: "2026-01-01"},
		{ID: "o-unk", Status: StatusOpen, Severity: severityUnknown},
		{ID: "o-a", Status: StatusOpen, Severity: severityLow, Found: "2026-03-01"},
		{ID: "o-crit-new", Status: StatusOpen, Severity: severityCritical, Found: "2026-02-01"},
	}
	Sort(fs)

	want := []string{"o-crit-new", "o-crit", "o-a", "o-b", "o-adv", "o-unk", "o-none", "f-high", "i-crit"}
	got := make([]string, len(fs))
	for i, f := range fs {
		got[i] = f.ID
	}
	if !slices.Equal(got, want) {
		t.Errorf("Sort order = %q, want %q", got, want)
	}
}

// A Facet outside Facets has no name, no value and no value order.
func TestFacetOutOfRange(t *testing.T) {
	t.Parallel()

	f := Facet(len(Facets))
	if got := f.String(); got != "" {
		t.Errorf("String() = %q, want empty", got)
	}
	if got := f.Of(Finding{Status: "open", Auditor: "a", Severity: "high", Category: "c"}); got != "" {
		t.Errorf("Of() = %q, want empty", got)
	}
	if got := f.order(); got != nil {
		t.Errorf("order() = %q, want nil", got)
	}
}
