package ftp

import (
	"strings"
	"testing"
)

func FuzzParseFeatures(f *testing.F) {
	// Add seed corpus for single line features
	f.Add("FEAT1\nFEAT2 params")
	f.Add("SIZE\nMDTM\nREST STREAM")
	f.Add("UTF8\nTVFS")

	f.Fuzz(func(t *testing.T, s string) {
		lines := strings.Split(s, "\n")
		// Just ensure it doesn't panic
		_ = parseFeatureLines(lines)
	})
}
