package cli

import (
	"sync"
	"testing"

	"github.com/knowledge-work/go-vow/internal/assert"
	"github.com/knowledge-work/go-vow/internal/tabletest"
)

func TestChangedFileSet_Empty(t *testing.T) {
	type emptyCase struct {
		set  ChangedFileSet
		want bool
	}

	tabletest.Run(t, map[string]emptyCase{
		"zero value is empty":                       {set: ChangedFileSet{}, want: true},
		"with-callers without files is still empty": {set: ChangedFileSet{WithCallers: true}, want: true},
		"populated files are non-empty":             {set: ChangedFileSet{Files: []string{"a.go"}}, want: false},
	}, func(t *testing.T, c emptyCase) {
		assert.Equal(t, "Empty()", c.set.Empty(), c.want)
	})
}

func TestActiveSet_RoundTrip(t *testing.T) {
	t.Cleanup(func() { SetActive(ChangedFileSet{}) })

	want := ChangedFileSet{Files: []string{"a.go", "b.go"}, WithCallers: true}
	SetActive(want)
	assert.DeepEqual(t, "Active()", Active(), want)
}

func TestActiveSet_ConcurrentAccess(t *testing.T) {
	// Asserts that concurrent SetActive / Active calls are race-free under -race.
	t.Cleanup(func() { SetActive(ChangedFileSet{}) })

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			SetActive(ChangedFileSet{Files: []string{"a.go"}})
			_ = Active()
		}()
	}
	wg.Wait()
}
