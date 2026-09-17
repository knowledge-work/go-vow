package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateChangedFiles checks that every path in files refers to an
// existing Go source file and returns the validated paths normalized to
// their absolute, cleaned form. A non-`.go` extension, a missing file, a
// directory, or any other stat failure surfaces as an error citing the
// offending path, so a malformed --changed-files argument exits the CLI
// before the analyzer runs.
func ValidateChangedFiles(files []string) ([]string, error) {
	normalized := make([]string, 0, len(files))
	for _, raw := range files {
		if !strings.HasSuffix(raw, ".go") {
			return nil, fmt.Errorf("path %q is not a Go source file", raw)
		}
		abs, err := filepath.Abs(raw)
		if err != nil {
			return nil, fmt.Errorf("path %q: %w", raw, err)
		}
		abs = filepath.Clean(abs)
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("path %q: %w", raw, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("path %q is a directory, not a file", raw)
		}
		normalized = append(normalized, abs)
	}
	return normalized, nil
}
