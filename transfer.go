package ftp

import (
	"fmt"
	"io"
	"net"
	"os"
)

// Store uploads data from an io.Reader to the remote path.
// The transfer is performed in binary mode (TYPE I).
//
// Example:
//
//	file, err := os.Open("local.txt")
//	if err != nil {
//	    return err
//	}
//	defer file.Close()
//
//	err = client.Store("remote.txt", file)
func (c *Client) Store(remotePath string, r io.Reader) error {
	// Set binary mode
	if err := c.Type("I"); err != nil {
		return fmt.Errorf("failed to set binary mode: %w", err)
	}

	// Open data connection and send STOR command
	dataConn, err := c.cmdDataConnFrom("STOR", remotePath)
	if err != nil {
		return err
	}

	// Copy data to the connection
	_, copyErr := io.Copy(dataConn, r)

	// Always finish the data connection (close and read response)
	finishErr := c.finishDataConn(dataConn)

	// Return the first error that occurred
	if copyErr != nil {
		return fmt.Errorf("upload failed: %w", copyErr)
	}
	if finishErr != nil {
		return finishErr
	}

	return nil
}

// StoreFrom uploads a local file to the remote path.
// This is a convenience wrapper around Store.
func (c *Client) StoreFrom(remotePath, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	return c.Store(remotePath, file)
}

// Retrieve downloads data from the remote path to an io.Writer.
// The transfer is performed in binary mode (TYPE I).
//
// Example:
//
//	file, err := os.Create("local.txt")
//	if err != nil {
//	    return err
//	}
//	defer file.Close()
//
//	err = client.Retrieve("remote.txt", file)
func (c *Client) Retrieve(remotePath string, w io.Writer) error {
	// Set binary mode
	if err := c.Type("I"); err != nil {
		return fmt.Errorf("failed to set binary mode: %w", err)
	}

	// Open data connection and send RETR command
	dataConn, err := c.cmdDataConnFrom("RETR", remotePath)
	if err != nil {
		return err
	}

	// Copy data from the connection
	_, copyErr := io.Copy(w, dataConn)

	// Always finish the data connection (close and read response)
	finishErr := c.finishDataConn(dataConn)

	// Return the first error that occurred
	if copyErr != nil {
		return fmt.Errorf("download failed: %w", copyErr)
	}
	if finishErr != nil {
		return finishErr
	}

	return nil
}

// RetrieveTo downloads a remote file to a local path.
// This is a convenience wrapper around Retrieve.
func (c *Client) RetrieveTo(remotePath, localPath string) error {
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer file.Close()

	return c.Retrieve(remotePath, file)
}

// Append appends data from an io.Reader to the remote path.
// If the file doesn't exist, it will be created.
// The transfer is performed in binary mode (TYPE I).
func (c *Client) Append(remotePath string, r io.Reader) error {
	// Set binary mode
	if err := c.Type("I"); err != nil {
		return fmt.Errorf("failed to set binary mode: %w", err)
	}

	// Open data connection and send APPE command
	dataConn, err := c.cmdDataConnFrom("APPE", remotePath)
	if err != nil {
		return err
	}

	// Copy data to the connection
	_, copyErr := io.Copy(dataConn, r)

	// Always finish the data connection (close and read response)
	finishErr := c.finishDataConn(dataConn)

	// Return the first error that occurred
	if copyErr != nil {
		return fmt.Errorf("append failed: %w", copyErr)
	}
	if finishErr != nil {
		return finishErr
	}

	return nil
}

// RestartAt sets the restart marker for the next transfer.
// This allows resuming a transfer from a specific byte offset.
// The offset applies to the next RETR or STOR command.
// This implements RFC 3959 - The FTP REST Extension.
//
// Example:
//
//	err := client.RestartAt(1024)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	err = client.Retrieve("file.bin", writer) // Resumes from byte 1024
func (c *Client) RestartAt(offset int64) error {
	resp, err := c.sendCommand("REST", fmt.Sprintf("%d", offset))
	if err != nil {
		return err
	}

	// REST should return 350 (Requested file action pending further information)
	if resp.Code != 350 {
		return &ProtocolError{
			Command:  "REST",
			Response: resp.Message,
			Code:     resp.Code,
		}
	}

	return nil
}

// RetrieveFrom downloads a file starting from the specified byte offset.
// This is useful for resuming interrupted downloads.
// The transfer is performed in binary mode (TYPE I).
//
// Example:
//
//	file, err := os.OpenFile("large.bin", os.O_WRONLY|os.O_APPEND, 0644)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer file.Close()
//
//	info, _ := file.Stat()
//	err = client.RetrieveFrom("large.bin", file, info.Size())
func (c *Client) RetrieveFrom(remotePath string, w io.Writer, offset int64) error {
	// Set binary mode
	if err := c.Type("I"); err != nil {
		return fmt.Errorf("failed to set binary mode: %w", err)
	}

	// Set restart marker if offset > 0
	if offset > 0 {
		if err := c.RestartAt(offset); err != nil {
			return fmt.Errorf("failed to set restart marker: %w", err)
		}
	}

	// Open data connection and send RETR command
	dataConn, err := c.cmdDataConnFrom("RETR", remotePath)
	if err != nil {
		return err
	}

	// Copy data from the connection
	_, copyErr := io.Copy(w, dataConn)

	// Always finish the data connection (close and read response)
	finishErr := c.finishDataConn(dataConn)

	// Return the first error that occurred
	if copyErr != nil {
		return fmt.Errorf("download failed: %w", copyErr)
	}
	if finishErr != nil {
		return finishErr
	}

	return nil
}

// StoreAt uploads a file starting from the specified byte offset.
// This allows resuming an interrupted upload by appending to an existing file.
// The transfer is performed in binary mode (TYPE I).
//
// Note: This uses APPE (append) mode when offset > 0, which may not be supported
// by all servers for resume functionality. For true resume support, the server
// must support REST+STOR, which is less common.
func (c *Client) StoreAt(remotePath string, r io.Reader, offset int64) error {
	// Set binary mode
	if err := c.Type("I"); err != nil {
		return fmt.Errorf("failed to set binary mode: %w", err)
	}

	var dataConn net.Conn
	var err error

	if offset > 0 {
		// Use APPE for resume (append mode)
		dataConn, err = c.cmdDataConnFrom("APPE", remotePath)
	} else {
		// Normal STOR
		dataConn, err = c.cmdDataConnFrom("STOR", remotePath)
	}

	if err != nil {
		return err
	}

	// Copy data to the connection
	_, copyErr := io.Copy(dataConn, r)

	// Always finish the data connection (close and read response)
	finishErr := c.finishDataConn(dataConn)

	// Return the first error that occurred
	if copyErr != nil {
		return fmt.Errorf("upload failed: %w", copyErr)
	}
	if finishErr != nil {
		return finishErr
	}

	return nil
}
