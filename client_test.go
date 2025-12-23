package ftp

import (
	"testing"
)

func TestParseFeatureLines_RFC2389(t *testing.T) {
	// RFC 2389 format with space-prefixed feature lines
	lines := []string{
		"211-Extensions supported:",
		" MLST size*;create;modify*;perm;media-type",
		" SIZE",
		" COMPRESSION",
		" MDTM",
		"211 END",
	}

	features := parseFeatureLines(lines)

	expected := map[string]string{
		"MLST":        "size*;create;modify*;perm;media-type",
		"SIZE":        "",
		"COMPRESSION": "",
		"MDTM":        "",
	}

	if len(features) != len(expected) {
		t.Errorf("expected %d features, got %d", len(expected), len(features))
	}

	for name, params := range expected {
		if gotParams, ok := features[name]; !ok {
			t.Errorf("missing feature %s", name)
		} else if gotParams != params {
			t.Errorf("feature %s: expected params %q, got %q", name, params, gotParams)
		}
	}
}
