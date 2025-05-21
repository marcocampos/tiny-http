package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/marcocampos/tiny-http/internal/server"
)

func main() {
	// Define command-line flags
	var (
		directory = flag.String("directory", "", "Directory to serve files from")
		hostname  = flag.String("hostname", "0.0.0.0", "Hostname or IP address to bind to")
		port      = flag.String("port", "8080", "Port to listen on")
		logLevel  = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	// Validate required flags
	if *directory == "" {
		fmt.Fprintln(os.Stderr, "error: directory flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Validate directory exists
	if _, err := os.Stat(*directory); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: directory %s does not exist\n", *directory)
		os.Exit(1)
	}

	// Setup logger
	logger := setupLogger(*logLevel)

	// Create server
	addr := fmt.Sprintf("%s:%s", *hostname, *port)
	srv := server.NewHTTPServer(addr, *directory, logger)

	if err := srv.ListenAndServe(context.Background()); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(level string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	return slog.New(handler)
}
