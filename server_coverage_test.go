package ftp_test

import (
	"testing"
	"time"

	"github.com/gonzalop/ftp"
)

func TestServerCoverage_AdditionalBranches(t *testing.T) {
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	if err := c.Login("anonymous", "anonymous"); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	// 1. Test CDUP
	if err := c.ChangeDir("/"); err != nil {
		t.Fatal(err)
	}
	if err := c.MakeDir("coverage_subdir"); err != nil {
		t.Fatal(err)
	}
	if err := c.ChangeDir("coverage_subdir"); err != nil {
		t.Fatal(err)
	}
	// Manual CDUP via Quote since client might not have it
	if _, err := c.Quote("CDUP"); err != nil {
		t.Errorf("CDUP failed: %v", err)
	}
	pwd, _ := c.CurrentDir()
	if pwd != "/" {
		t.Errorf("Expected /, got %s", pwd)
	}

	// 2. Test MODE
	modes := []struct {
		mode string
		code int
	}{
		{"S", 200},
		{"B", 504},
		{"C", 504},
		{"X", 504},
	}
	for _, m := range modes {
		resp, _ := c.Quote("MODE", m.mode)
		if resp.Code != m.code {
			t.Errorf("MODE %s expected %d, got %d", m.mode, m.code, resp.Code)
		}
	}

	// 3. Test STRU
	structures := []struct {
		stru string
		code int
	}{
		{"F", 200},
		{"R", 504},
		{"P", 504},
		{"X", 504},
	}
	for _, s := range structures {
		resp, _ := c.Quote("STRU", s.stru)
		if resp.Code != s.code {
			t.Errorf("STRU %s expected %d, got %d", s.stru, s.code, resp.Code)
		}
	}

	// 4. Test TYPE (ASCII and invalid)
	if _, err := c.Quote("TYPE", "A"); err != nil {
		t.Errorf("TYPE A failed: %v", err)
	}
	if resp, _ := c.Quote("TYPE", "E"); resp.Code != 504 {
		t.Errorf("TYPE E should be 504, got %d", resp.Code)
	}

	// 5. Test SITE error cases
	if resp, _ := c.Quote("SITE"); resp.Code != 501 {
		t.Errorf("SITE empty should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("SITE", "UNKNOWN"); resp.Code != 502 {
		t.Errorf("SITE UNKNOWN should be 502, got %d", resp.Code)
	}
	if resp, _ := c.Quote("SITE", "CHMOD"); resp.Code != 501 {
		t.Errorf("SITE CHMOD empty should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("SITE", "CHMOD", "999", "file"); resp.Code != 501 {
		t.Errorf("SITE CHMOD invalid mode should be 501, got %d", resp.Code)
	}

	// 6. Test STAT with path (not implemented)
	if resp, _ := c.Quote("STAT", "/"); resp.Code != 502 {
		t.Errorf("STAT with path should be 502, got %d", resp.Code)
	}

	// 7. Test HELP with arg
	if resp, _ := c.Quote("HELP", "USER"); resp.Code != 214 {
		t.Errorf("HELP USER should be 214, got %d", resp.Code)
	}

	// 8. Test PORT/EPRT error cases
	if resp, _ := c.Quote("PORT", "invalid"); resp.Code != 501 {
		t.Errorf("PORT invalid should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("EPRT", "invalid"); resp.Code != 501 {
		t.Errorf("EPRT invalid should be 501, got %d", resp.Code)
	}
	if resp, _ := c.Quote("EPRT", "|1|127.0.0.1|65536|"); resp.Code != 501 {
		t.Errorf("EPRT invalid port should be 501, got %d", resp.Code)
	}

	// 9. Test REST error case
	if resp, _ := c.Quote("REST", "invalid"); resp.Code != 501 {
		t.Errorf("REST invalid should be 501, got %d", resp.Code)
	}
}

func TestServerCoverage_NoAuth(t *testing.T) {
	addr, cleanup, _ := setupServer(t)
	defer cleanup()

	c, err := ftp.Dial(addr, ftp.WithTimeout(5*time.Second))
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer func() {
		if err := c.Quit(); err != nil {
			t.Logf("Quit failed: %v", err)
		}
	}()

	// Test commands without login
	cmds := []string{"PWD", "CWD", "LIST", "RETR", "STOR", "SIZE", "MDTM", "MLSD", "MLST"}
	for _, cmd := range cmds {
		resp, _ := c.Quote(cmd, "path")
		if resp.Code != 530 {
			t.Errorf("%s without login expected 530, got %d", cmd, resp.Code)
		}
	}
}
