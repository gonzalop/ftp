package server

import "testing"

func fatalIfErr(t *testing.T, err error, format string, args ...interface{}) {
	t.Helper()
	if err != nil {
		t.Fatalf(format+": %v", append(args, err)...)
	}
}
