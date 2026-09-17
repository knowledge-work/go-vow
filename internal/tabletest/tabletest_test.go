package tabletest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
)

func TestRunVisitsEveryCaseInSortedOrder(t *testing.T) {
	var visited []string
	Run(t, map[string]int{"zeta": 3, "alpha": 1, "mid": 2}, func(t *testing.T, value int) {
		visited = append(visited, subtestName(t))
	})

	assert.DeepEqual(t, "visited", visited, []string{"alpha", "mid", "zeta"})
}

func TestRunPassesTheCaseKeyedByItsName(t *testing.T) {
	reached := map[string]int{}
	Run(t, map[string]int{"one": 1, "two": 2}, func(t *testing.T, value int) {
		reached[subtestName(t)] = value
	})

	assert.DeepEqual(t, "cases reached", reached, map[string]int{"one": 1, "two": 2})
}

// TestRunStopsOnAnEmptyTable runs the empty-table check in a subprocess,
// because the check reports on the *testing.T it is handed and a test
// cannot observe its own failure.
func TestRunStopsOnAnEmptyTable(t *testing.T) {
	if os.Getenv(emptyTableFixtureVar) == "1" {
		Run(t, map[string]int{}, func(t *testing.T, value int) {
			t.Error("the run function must not be called for an empty table")
		})
		return
	}

	fixture := exec.Command(os.Args[0], "-test.run=TestRunStopsOnAnEmptyTable", "-test.v")
	fixture.Env = append(os.Environ(), emptyTableFixtureVar+"=1")
	out, err := fixture.CombinedOutput()

	assert.Error(t, "fixture run", err)
	assert.Contains(t, "fixture output", string(out), "no cases")
}

// emptyTableFixtureVar tells a re-executed test binary to run the empty
// table instead of the check around it.
const emptyTableFixtureVar = "VOW_TABLETEST_EMPTY_FIXTURE"

// subtestName returns the case name Run derived the subtest from.
func subtestName(t *testing.T) string {
	t.Helper()
	return t.Name()[strings.LastIndex(t.Name(), "/")+1:]
}
