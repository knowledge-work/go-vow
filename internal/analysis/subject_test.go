package analysis

import (
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/dsl"
)

// TestAnnotationMarkers_dropsOtherSchemes pins the scheme filter:
// AnnotationMarker reports "" for a subject written in any scheme other
// than "annotation:", and such a subject must not reach the marker list as
// an empty entry. Nothing else in the suite covers a preset that mixes
// schemes, so without this the filter can be deleted while every test
// stays green.
func TestAnnotationMarkers_dropsOtherSchemes(t *testing.T) {
	preset := &dsl.Preset{Subjects: []dsl.Subject{
		{Match: "annotation: vow:define @Sentinel"},
		{Match: "name:Foo*"},
		{Match: "annotation:vow:define @Other"},
	}}

	assert.DeepEqual(t, "annotationMarkers", annotationMarkers(preset),
		[]string{"vow:define @Sentinel", "vow:define @Other"})
}

// TestAnnotationMarkers_allOtherSchemes pins the empty case by length
// rather than by value. findSubjects reads the markers themselves once it
// has any, but on an empty list it only checks the length before bailing
// out, so whether that list is nil or empty is incidental.
func TestAnnotationMarkers_allOtherSchemes(t *testing.T) {
	preset := &dsl.Preset{Subjects: []dsl.Subject{{Match: "name:Foo*"}}}

	assert.Len(t, "annotationMarkers", annotationMarkers(preset), 0)
}
