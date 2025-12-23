package ftp

import (
	"testing"
)

func FuzzParseListLine(f *testing.F) {
	// Add seed corpus
	f.Add("-rw-r--r--   1 user  group     1024 Dec 20 10:30 file.txt")
	f.Add("drwxr-xr-x   2 user  group     4096 Dec 20 10:30 mydir")
	f.Add("09-24-24  10:30AM       <DIR>          logger")
	f.Add("12-14-23  12:22PM           1037794 large-document.pdf")
	f.Add("+i8388621.48594,m825718503,r,s280,\tdjb.html")
	f.Add("+/,m824255907\tdata")

	f.Fuzz(func(t *testing.T, line string) {
		// Just ensure it doesn't panic
		_ = parseListLine(line, nil)
	})
}
