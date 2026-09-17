// Package dedup exists so the driver test can load it with
// Tests: true and see the same diagnostic surface through both
// the base package root and its "[.test]" augmentation.
package dedup

// Twin is the site the test analyzer reports every time it visits
// the package. Loading dedup with Tests: true builds it twice
// (once as dedup, once as dedup [dedup.test]); after
// deduplication the printed output must contain the diagnostic
// only once.
func Twin() {}
