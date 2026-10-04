// Package configSyntax is the analysistest fixture for a vow.yaml that is
// not valid YAML, whose parse error is a single line.
package configSyntax // want `vow\[config\]: parse .*/configSyntax/vow\.yaml: yaml: line 1: did not find expected node content; the built-in defaults apply instead`

func one() int { return 1 }
