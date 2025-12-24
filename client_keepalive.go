package ftp

import (
	"sync/atomic"
	"time"
)

// startKeepAlive starts a goroutine that sends NOOP commands
// if the connection has been idle for the configured idleTimeout.
func (c *Client) startKeepAlive() {
	if c.idleTimeout == 0 {
		return
	}

	c.quitChan = make(chan struct{})

	// We use a ticker that runs at half the idle timeout to be safe
	ticker := time.NewTicker(c.idleTimeout / 2)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Skip if a data transfer is in progress
				if atomic.LoadInt32(&c.transferInProgress) == 1 {
					continue
				}

				c.mu.Lock()
				last := c.lastCommand
				c.mu.Unlock()

				// If time since last command is greater than idle timeout, send NOOP
				if time.Since(last) >= c.idleTimeout {
					if c.logger != nil {
						c.logger.Debug("sending keep-alive NOOP")
					}
					// Ignore errors (connection might be closed)
					_ = c.Noop()
				}
			case <-c.quitChan:
				return
			}
		}
	}()
}
