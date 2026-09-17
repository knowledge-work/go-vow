// Package importUnknown exercises the unknown vow:import preset-path
// diagnostic.
//
// vow:import bogus "preset/no-such-thing" // want `vow\[sentinel-error\]: vow:import alias "bogus" references unknown preset path "preset/no-such-thing"`
package importUnknown
