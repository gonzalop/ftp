module github.com/gonzalop/ftp/examples/quic

go 1.25

// Use local FTP library
replace github.com/gonzalop/ftp => ../..

require (
	github.com/gonzalop/ftp v0.0.0-00010101000000-000000000000
	github.com/quic-go/quic-go v0.58.0
)

require (
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
)
