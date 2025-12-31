package server

// Predefined command groups for use with WithDisableCommands.
// These provide convenient ways to disable categories of FTP commands.
//
// Example usage:
//
//	// Disable active mode for QUIC transport
//	srv, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithListenerFactory(&QuicListenerFactory{...}),
//	    server.WithDisableCommands(server.ActiveModeCommands...),
//	)
//
//	// Create a read-only server
//	srv, _ := server.NewServer(":21",
//	    server.WithDriver(driver),
//	    server.WithDisableCommands(server.WriteCommands...),
//	)
var (
	// LegacyCommands contains deprecated X* command variants from RFC 775.
	// These are legacy aliases that modern FTP clients don't need.
	//
	// Commands: XCWD, XCUP, XPWD, XMKD, XRMD
	//
	// Use case: Disable legacy commands to simplify server implementation
	// and reduce attack surface.
	LegacyCommands = []string{
		"XCWD", // Use CWD instead
		"XCUP", // Use CDUP instead
		"XPWD", // Use PWD instead
		"XMKD", // Use MKD instead
		"XRMD", // Use RMD instead
	}

	// ActiveModeCommands contains commands for active mode data connections.
	//
	// Commands: PORT, EPRT
	//
	// Use case: Disable these for alternative transports (e.g., QUIC) that only
	// support passive mode, or for security hardening.
	ActiveModeCommands = []string{
		"PORT", // Active mode for IPv4
		"EPRT", // Active mode for IPv6
	}

	// WriteCommands contains all commands that modify the filesystem.
	//
	// Commands: STOR, APPE, STOU, DELE, RMD, XRMD, MKD, XMKD, RNFR, RNTO
	//
	// Use case: Disable these to create a read-only FTP server for distribution
	// of files without allowing uploads or modifications.
	//
	// Note: For per-user read-only access, use the FSDriver's authenticator
	// function to return readOnly=true for specific users instead.
	WriteCommands = []string{
		"STOR", // Store file
		"APPE", // Append to file
		"STOU", // Store unique
		"DELE", // Delete file
		"RMD",  // Remove directory
		"XRMD", // Remove directory (legacy)
		"MKD",  // Make directory
		"XMKD", // Make directory (legacy)
		"RNFR", // Rename from
		"RNTO", // Rename to
	}

	// SiteCommands contains SITE administrative commands.
	//
	// Commands: SITE
	//
	// Use case: Disable to restrict administrative operations like SITE CHMOD.
	SiteCommands = []string{
		"SITE", // All SITE commands
	}
)
