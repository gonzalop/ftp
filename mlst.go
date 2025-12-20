package ftp

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// MLEntry represents a machine-readable directory entry from MLST/MLSD commands.
// This provides structured, unambiguous file information compared to LIST.
type MLEntry struct {
	// Name is the file or directory name
	Name string

	// Type is the entry type: "file", "dir", "cdir" (current), "pdir" (parent), or "link"
	Type string

	// Size is the file size in bytes (0 for directories)
	Size int64

	// ModTime is the modification time
	ModTime time.Time

	// Perm contains permission information (e.g., "r", "w", "a", "d", "f")
	Perm string

	// UnixMode is the Unix file mode (if provided by server)
	UnixMode string

	// Facts contains all raw facts from the server
	Facts map[string]string
}

// MLStat returns information about a single file or directory using the MLST command.
// This implements RFC 3659 - Extensions to FTP.
//
// Example:
//
//	entry, err := client.MLStat("file.txt")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Size: %d, Modified: %s\n", entry.Size, entry.ModTime)
func (c *Client) MLStat(path string) (*MLEntry, error) {
	resp, err := c.sendCommand("MLST", path)
	if err != nil {
		return nil, err
	}

	if resp.Code != 250 {
		return nil, &ProtocolError{
			Command:  "MLST",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// MLST returns a multi-line response with the entry on the second line
	// Format: "250-Listing path\n facts entry-name\n250 End"
	if len(resp.Lines) < 2 {
		return nil, fmt.Errorf("invalid MLST response: too few lines")
	}

	// Find the line with the entry (starts with a space)
	var entryLine string
	for _, line := range resp.Lines {
		trimmed := strings.TrimSpace(line)
		// Skip status lines
		if len(line) >= 4 && (line[3] == '-' || line[3] == ' ') {
			continue
		}
		// This should be the entry line
		if trimmed != "" {
			entryLine = trimmed
			break
		}
	}

	if entryLine == "" {
		return nil, fmt.Errorf("no entry found in MLST response")
	}

	entry, err := parseMLEntry(entryLine)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MLST entry: %w", err)
	}

	return entry, nil
}

// MLList returns a machine-readable directory listing using the MLSD command.
// This implements RFC 3659 - Extensions to FTP.
//
// Example:
//
//	entries, err := client.MLList("/pub")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, entry := range entries {
//	    fmt.Printf("%s: %d bytes\n", entry.Name, entry.Size)
//	}
func (c *Client) MLList(path string) ([]*MLEntry, error) {
	// Open data connection and send MLSD command
	var dataConn net.Conn
	var err error

	if path == "" {
		dataConn, err = c.cmdDataConnFrom("MLSD")
	} else {
		dataConn, err = c.cmdDataConnFrom("MLSD", path)
	}
	if err != nil {
		return nil, err
	}

	// Read the directory listing
	var entries []*MLEntry
	scanner := bufio.NewScanner(dataConn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		entry, parseErr := parseMLEntry(line)
		if parseErr != nil {
			// Skip malformed entries but continue processing
			continue
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		dataConn.Close()
		return nil, fmt.Errorf("failed to read directory listing: %w", err)
	}

	// Finish the data connection
	if err := c.finishDataConn(dataConn); err != nil {
		return nil, err
	}

	return entries, nil
}

// parseMLEntry parses a single MLST/MLSD entry line.
// Format: "facts entry-name"
// Facts format: "fact1=value1;fact2=value2;fact3=value3; "
func parseMLEntry(line string) (*MLEntry, error) {
	// Find the space that separates facts from the name
	spaceIdx := strings.Index(line, " ")
	if spaceIdx == -1 {
		return nil, fmt.Errorf("invalid ML entry format: no space separator")
	}

	factsStr := line[:spaceIdx]
	name := line[spaceIdx+1:]

	// Parse facts
	facts := make(map[string]string)
	factPairs := strings.Split(factsStr, ";")
	for _, pair := range factPairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		factName := strings.ToLower(parts[0])
		factValue := parts[1]
		facts[factName] = factValue
	}

	// Build the entry
	entry := &MLEntry{
		Name:  name,
		Facts: facts,
	}

	// Extract common facts
	if typeVal, ok := facts["type"]; ok {
		entry.Type = strings.ToLower(typeVal)
	}

	if sizeVal, ok := facts["size"]; ok {
		if size, err := strconv.ParseInt(sizeVal, 10, 64); err == nil {
			entry.Size = size
		}
	}

	if modifyVal, ok := facts["modify"]; ok {
		// Format: YYYYMMDDHHMMSS or YYYYMMDDHHMMSS.sss
		// Remove fractional seconds if present
		timestamp := strings.Split(modifyVal, ".")[0]
		if len(timestamp) == 14 {
			if modTime, err := time.Parse("20060102150405", timestamp); err == nil {
				entry.ModTime = modTime.UTC()
			}
		}
	}

	if permVal, ok := facts["perm"]; ok {
		entry.Perm = permVal
	}

	if modeVal, ok := facts["unix.mode"]; ok {
		entry.UnixMode = modeVal
	}

	return entry, nil
}
