package util

import (
	"log/slog"
	"os"
)

var (
	// Logger is the shared structured logger for all packages
	Logger *slog.Logger
)

// SetVerbose configures the logger level based on verbose mode
// If enabled is nil, checks BLOCKYMERGE_VERBOSE environment variable
func SetVerbose(enabled *bool) {
	var verbose bool
	if enabled != nil {
		verbose = *enabled
	} else {
		// Check environment variable
		verbose = os.Getenv("BLOCKYMERGE_VERBOSE") != ""
	}

	// Create logger with appropriate level
	if verbose {
		// Verbose mode: show all logs (Debug and above)
		opts := &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
		Logger = slog.New(slog.NewTextHandler(os.Stdout, opts))
	} else {
		// Non-verbose mode: show only warnings and errors
		opts := &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}
		Logger = slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
}

func init() {
	// Initialize logger with default level (Info)
	SetVerbose(nil)
}
