package main

import (
	"cmp"
	"encoding/json"
	"fmt"

	"golang.org/x/tools/go/analysis"

	"github.com/knowledge-work/go-vow/internal/driver"
	"github.com/knowledge-work/go-vow/internal/seq"
)

// driverReport converts a run's result into the `go vet -json` shape that
// -json prints, keyed by package ID and then analyzer name. It records
// every job's error and, for root jobs only, the diagnostics. A job with
// no error and no root diagnostics is left out.
//
// vow:nil (!) !
func driverReport(r *driver.Result) map[string]map[string]vetResult {
	report := map[string]map[string]vetResult{}
	for j := range r.All() {
		var result vetResult
		switch {
		case j.Err != nil:
			result.Err = j.Err.Error()
		case j.Root && len(j.Diagnostics) > 0:
			fset := j.Package.Fset
			result.Diagnostics = seq.ChainOf(j.Diagnostics...).Map(func(d analysis.Diagnostic) vetDiagnostic {
				return vetDiagnostic{
					Category: d.Category,
					Posn:     fset.Position(d.Pos).String(),
					End:      fset.Position(cmp.Or(d.End, d.Pos)).String(),
					Message:  d.Message,
					Related: seq.ChainOf(d.Related...).Map(func(rel analysis.RelatedInformation) vetRelated {
						return vetRelated{
							Posn:    fset.Position(rel.Pos).String(),
							End:     fset.Position(cmp.Or(rel.End, rel.Pos)).String(),
							Message: rel.Message,
						}
					}).ToSlice(),
				}
			}).ToSlice()
		default:
			continue
		}
		if report[j.Package.ID] == nil {
			report[j.Package.ID] = map[string]vetResult{}
		}
		report[j.Package.ID][j.Analyzer.Name] = result
	}
	return report
}

// jsonReport renders a report as -json prints it: one indented JSON
// document followed by a newline, keys sorted. An empty or nil report
// renders as `{}`, so a run with nothing to say still prints a document.
//
// vow:nil (?) !
func jsonReport(report map[string]map[string]vetResult) []byte {
	if report == nil {
		// A nil map would encode as `null`.
		report = map[string]map[string]vetResult{}
	}
	data, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		// The report holds only strings, in structs, slices and maps,
		// which encoding/json cannot fail to encode.
		panic(fmt.Sprintf("vow: marshal JSON report: %v", err))
	}
	return append(data, '\n')
}
