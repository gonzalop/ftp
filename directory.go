package ftp

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

// Entry represents a file or directory entry from a LIST command.
type Entry struct {
	Name   string
	Type   string // "file", "dir", or "link"
	Size   int64
	Target string // For symlinks, the target path (empty for files/dirs)
	Raw    string // The raw line from the LIST command
}

// List returns a list of files and directories in the specified path.
// If path is empty, it lists the current directory.
//
// The parser supports multiple directory listing formats for maximum compatibility:
//
//   - Unix-style (9-field): perms links owner group size month day time/year name
//   - Unix-style (8-field): perms links owner size month day time/year name (no group)
//   - Unix-style (numeric): 644 links owner group size month day time/year name
//   - DOS/Windows: MM-DD-YY HH:MMAM/PM size|<DIR> filename
//   - EPLF: +facts\tname or +facts name
//
// For standardized, machine-readable listings, use MLList instead (requires MLSD support).
//
// Example:
//
//	entries, err := client.List("/pub")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for _, entry := range entries {
//	    fmt.Printf("%s: %d bytes (%s)\n", entry.Name, entry.Size, entry.Type)
//	    if entry.Type == "link" && entry.Target != "" {
//	        fmt.Printf("  -> %s\n", entry.Target)
//	    }
//	}
func (c *Client) List(path string) ([]*Entry, error) {
	// Open data connection and send LIST command
	var dataConn net.Conn
	var err error

	if path == "" {
		dataConn, err = c.cmdDataConnFrom("LIST")
	} else {
		dataConn, err = c.cmdDataConnFrom("LIST", path)
	}
	if err != nil {
		return nil, err
	}

	// Read the directory listing
	var entries []*Entry
	scanner := bufio.NewScanner(dataConn)
	for scanner.Scan() {
		line := scanner.Text()
		entry := parseListLine(line)
		if entry != nil {
			entries = append(entries, entry)
		}
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

// parseListLine parses a single line from a LIST response.
// This parser handles Unix-style, DOS/Windows-style, and EPLF formats.
func parseListLine(line string) *Entry {
	// Handle empty or whitespace-only lines
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		if line != "" {
			slog.Debug("Skipping whitespace-only line",
				"raw", line)
		}
		return nil
	}

	entry := &Entry{Raw: line}

	// Try EPLF format first (starts with '+')
	// Example: "+i8388621.48594,m825718503,r,s280,\tdjb.html"
	if strings.HasPrefix(trimmed, "+") {
		if parseEPLFEntry(entry, trimmed) {
			return entry
		}
		slog.Debug("EPLF format detected but parsing failed",
			"raw", line)
	}

	fields := strings.Fields(line)

	// Try DOS/Windows-style listing
	// Example: "12-14-23  12:22PM           1037794 large-document.pdf"
	// Example: "09-24-24  10:30AM       <DIR>          logger"
	// Format: date time size-or-<DIR> filename
	if len(fields) >= 4 && isDOSDate(fields[0]) {
		if parseDOSEntry(entry, fields) {
			return entry
		}
		// DOS date detected but parsing failed
		slog.Debug("DOS date format detected but parsing failed",
			"raw", line,
			"fields", fields)
	}

	// Try Unix-style listing
	// Supports both 9-field (with group) and 8-field (without group) formats
	// Also handles numeric permissions (e.g., "644" instead of "-rw-r--r--")
	if len(fields) >= 8 {
		if parseUnixEntry(entry, fields) {
			return entry
		}
	}

	// If we can't parse it, just return the raw line as the name
	slog.Debug("Unable to parse LIST line, unknown format",
		"raw", line,
		"field_count", len(fields))
	entry.Name = line
	entry.Type = "unknown"
	return entry
}

// parseUnixEntry parses a Unix-style directory entry.
// Handles both 9-field and 8-field formats, numeric and symbolic permissions.
func parseUnixEntry(entry *Entry, fields []string) bool {
	// Determine if first field is permissions (symbolic or numeric)
	perms := fields[0]

	// Check for symbolic permissions (starts with -, d, l, etc.)
	isSymbolic := len(perms) >= 1 && (perms[0] == '-' || perms[0] == 'd' ||
		perms[0] == 'l' || perms[0] == 'b' || perms[0] == 'c' ||
		perms[0] == 'p' || perms[0] == 's')

	// Check for numeric permissions (3-4 digits)
	isNumeric := len(perms) >= 3 && len(perms) <= 4
	for _, ch := range perms {
		if ch < '0' || ch > '7' {
			isNumeric = false
			break
		}
	}

	if !isSymbolic && !isNumeric {
		return false
	}

	// Determine type from permissions
	if isSymbolic {
		if perms[0] == 'd' {
			entry.Type = "dir"
		} else if perms[0] == 'l' {
			entry.Type = "link"
		} else {
			entry.Type = "file"
		}
	} else {
		// Numeric permissions - assume file (can't determine type)
		entry.Type = "file"
	}

	// Determine field layout: 9-field or 8-field format
	// 9-field: perms links owner group size month day time/year name
	// 8-field: perms links owner size month day time/year name
	var sizeIdx, nameStartIdx int

	if len(fields) >= 9 {
		// Try 9-field format first
		if _, err := parseSize(fields[4]); err == nil {
			sizeIdx = 4
			nameStartIdx = 8
		} else if len(fields) >= 8 {
			// Try 8-field format (no group)
			if _, err := parseSize(fields[3]); err == nil {
				sizeIdx = 3
				nameStartIdx = 7
			} else {
				return false
			}
		} else {
			return false
		}
	} else if len(fields) >= 8 {
		// Only 8 fields, try 8-field format
		if _, err := parseSize(fields[3]); err == nil {
			sizeIdx = 3
			nameStartIdx = 7
		} else {
			return false
		}
	} else {
		return false
	}

	// Parse size
	if size, err := parseSize(fields[sizeIdx]); err == nil {
		entry.Size = size
	} else {
		slog.Debug("Failed to parse size in Unix format",
			"raw", entry.Raw,
			"size_field", fields[sizeIdx],
			"error", err)
		return false
	}

	// Name is everything after nameStartIdx
	fullName := strings.Join(fields[nameStartIdx:], " ")

	// For links, extract the actual name and target (format: "name -> target")
	if entry.Type == "link" {
		// Use " -> " as separator (note the spaces)
		if idx := strings.Index(fullName, " -> "); idx != -1 {
			entry.Name = fullName[:idx]
			entry.Target = fullName[idx+4:] // Skip " -> "
		} else {
			// Fallback: no arrow found, just use the full name
			slog.Debug("Symlink detected but no arrow separator found",
				"raw", entry.Raw,
				"fullname", fullName)
			entry.Name = fullName
		}
	} else {
		entry.Name = fullName
	}

	return true
}

// parseEPLFEntry parses an EPLF (Easily Parsed LIST Format) entry.
// Format: +facts\tname or +facts name
// Facts are comma-separated, e.g.: i=inode, m=mtime, s=size, /, r, etc.
// Example: "+i8388621.48594,m825718503,r,s280,\tdjb.html"
func parseEPLFEntry(entry *Entry, line string) bool {
	if !strings.HasPrefix(line, "+") {
		return false
	}

	// Remove the leading '+'
	line = line[1:]

	// Find the separator (tab or space after facts)
	var name string
	var facts string

	if idx := strings.IndexAny(line, "\t "); idx != -1 {
		facts = line[:idx]
		name = strings.TrimSpace(line[idx+1:])
	} else {
		// No separator found
		return false
	}

	if name == "" {
		return false
	}

	entry.Name = name
	entry.Type = "file" // Default to file

	// Parse facts (comma-separated)
	for _, fact := range strings.Split(facts, ",") {
		if fact == "" {
			continue
		}

		switch fact[0] {
		case '/':
			// Directory
			entry.Type = "dir"
		case 's':
			// Size
			if len(fact) > 1 {
				if size, err := parseSize(fact[1:]); err == nil {
					entry.Size = size
				}
			}
			// Other facts (i=inode, m=mtime, r=readable, etc.) are ignored for now
		}
	}

	return true
}

// isDOSDate checks if a string looks like a DOS/Windows date format.
// Common formats: MM-DD-YY, MM-DD-YYYY, MM/DD/YY, MM/DD/YYYY
func isDOSDate(s string) bool {
	// Try both dash and slash separators
	var parts []string
	if strings.Contains(s, "-") {
		parts = strings.Split(s, "-")
	} else if strings.Contains(s, "/") {
		parts = strings.Split(s, "/")
	} else {
		return false
	}

	if len(parts) != 3 {
		return false
	}

	// Validate each part is numeric and has reasonable length
	// Month: 1-2 digits, Day: 1-2 digits, Year: 2 or 4 digits
	for i, part := range parts {
		if len(part) < 1 || len(part) > 4 {
			return false
		}
		// Year can be 2 or 4 digits
		if i == 2 && len(part) != 2 && len(part) != 4 {
			return false
		}
		// Month and day should be 1-2 digits
		if i < 2 && len(part) > 2 {
			return false
		}
		// All parts must be numeric
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

// parseDOSEntry parses a DOS/Windows-style directory entry.
// Returns true if parsing was successful, false otherwise.
func parseDOSEntry(entry *Entry, fields []string) bool {
	// DOS format: date time size-or-<DIR> filename...
	// Example: "12-14-23  12:22PM           1037794 large-document.pdf"
	// Example: "09-24-24  10:30AM       <DIR>          logger"

	if len(fields) < 4 {
		return false
	}

	// Check if it's a directory
	if fields[2] == "<DIR>" {
		entry.Type = "dir"
		entry.Size = 0
		entry.Name = strings.Join(fields[3:], " ")
		return true
	}

	// It's a file - parse the size
	size, err := parseSize(fields[2])
	if err != nil {
		slog.Debug("Failed to parse size in DOS format",
			"raw", entry.Raw,
			"size_field", fields[2],
			"error", err)
		return false
	}

	entry.Type = "file"
	entry.Size = size
	entry.Name = strings.Join(fields[3:], " ")
	return true
}

// parseSize parses a size string from a directory listing.
func parseSize(sizeStr string) (int64, error) {
	return strconv.ParseInt(sizeStr, 10, 64)
}

// NameList returns a simple list of file and directory names in the specified path.
// This uses the NLST command which returns just names, one per line.
func (c *Client) NameList(path string) ([]string, error) {
	// Open data connection and send NLST command
	var dataConn net.Conn
	var err error

	if path == "" {
		dataConn, err = c.cmdDataConnFrom("NLST")
	} else {
		dataConn, err = c.cmdDataConnFrom("NLST", path)
	}
	if err != nil {
		return nil, err
	}

	// Read the name list
	var names []string
	scanner := bufio.NewScanner(dataConn)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			names = append(names, name)
		}
	}

	if err := scanner.Err(); err != nil {
		dataConn.Close()
		return nil, fmt.Errorf("failed to read name list: %w", err)
	}

	// Finish the data connection
	if err := c.finishDataConn(dataConn); err != nil {
		return nil, err
	}

	return names, nil
}

// ChangeDir changes the current working directory.
func (c *Client) ChangeDir(path string) error {
	_, err := c.expect2xx("CWD", path)
	return err
}

// CurrentDir returns the current working directory.
func (c *Client) CurrentDir() (string, error) {
	resp, err := c.expect2xx("PWD")
	if err != nil {
		return "", err
	}

	// Parse the directory from the response
	// Example: 257 "/home/user" is the current directory
	msg := resp.Message
	start := strings.Index(msg, "\"")
	if start == -1 {
		return "", fmt.Errorf("invalid PWD response: %s", msg)
	}
	end := strings.Index(msg[start+1:], "\"")
	if end == -1 {
		return "", fmt.Errorf("invalid PWD response: %s", msg)
	}

	return msg[start+1 : start+1+end], nil
}

// MakeDir creates a new directory.
func (c *Client) MakeDir(path string) error {
	_, err := c.expect2xx("MKD", path)
	return err
}

// RemoveDir removes a directory.
func (c *Client) RemoveDir(path string) error {
	_, err := c.expect2xx("RMD", path)
	return err
}

// Delete deletes a file.
func (c *Client) Delete(path string) error {
	_, err := c.expect2xx("DELE", path)
	return err
}

// Rename renames a file or directory.
func (c *Client) Rename(from, to string) error {
	// Send RNFR (rename from)
	resp, err := c.sendCommand("RNFR", from)
	if err != nil {
		return err
	}

	if resp.Code != 350 {
		return &ProtocolError{
			Command:  "RNFR",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	// Send RNTO (rename to)
	_, err = c.expect2xx("RNTO", to)
	return err
}

// Size returns the size of a file in bytes.
func (c *Client) Size(path string) (int64, error) {
	resp, err := c.expect2xx("SIZE", path)
	if err != nil {
		return 0, err
	}

	// Parse the size from the response
	var size int64
	_, parseErr := fmt.Sscanf(resp.Message, "%d", &size)
	if parseErr != nil {
		return 0, fmt.Errorf("invalid SIZE response: %s", resp.Message)
	}

	return size, nil
}

// ModTime returns the modification time of a file using the MDTM command.
// This implements RFC 3659 - Extensions to FTP.
//
// Example:
//
//	modTime, err := client.ModTime("file.txt")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Last modified: %s\n", modTime)
func (c *Client) ModTime(path string) (time.Time, error) {
	resp, err := c.expect2xx("MDTM", path)
	if err != nil {
		return time.Time{}, err
	}

	// Parse the timestamp from the response
	// Format: YYYYMMDDHHMMSS (e.g., "20231220143000" for Dec 20, 2023 14:30:00)
	timestamp := strings.TrimSpace(resp.Message)
	if len(timestamp) != 14 {
		return time.Time{}, fmt.Errorf("invalid MDTM response format: %s", resp.Message)
	}

	// Parse using the FTP timestamp format
	modTime, parseErr := time.Parse("20060102150405", timestamp)
	if parseErr != nil {
		return time.Time{}, fmt.Errorf("failed to parse MDTM timestamp: %w", parseErr)
	}

	return modTime.UTC(), nil
}
