package findings

// Values that recur across this package's tests. The store's own vocabulary
// (states and severities) comes from facet.go.
const (
	lensAudit    = "audit"
	lensSecurity = "security"
	lensCI       = "ci"

	catCorrectness = "correctness"
	catDocs        = "docs"
	catSecurity    = "security"

	severityUnknown = "urgent"

	idA1 = "a-1"
	idA2 = "a-2"
	idS1 = "s-1"
	idS2 = "s-2"
	idC1 = "c-1"
	idC2 = "c-2"
)
