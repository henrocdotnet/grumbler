package stages

// defaultConcurrency is used whenever review.concurrency is unset or invalid.
// Batches are deliberately small, so there are many of them; concurrency is
// what keeps wall clock down.
const defaultConcurrency = 10

// effectiveConcurrency clamps a configured concurrency to a usable limit.
//
// errgroup.SetLimit(0) blocks every Go call forever, so a hand-written
// `concurrency: 0` in YAML would deadlock the run. Callers must route the
// configured value through here before calling SetLimit.
func effectiveConcurrency(n int) int {
	if n <= 0 {
		return defaultConcurrency
	}
	return n
}
