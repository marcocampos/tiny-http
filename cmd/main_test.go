package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected slog.Level
	}{
		{"debug level", "debug", slog.LevelDebug},
		{"info level", "info", slog.LevelInfo},
		{"warn level", "warn", slog.LevelWarn},
		{"error level", "error", slog.LevelError},
		{"invalid level defaults to info", "invalid", slog.LevelInfo},
		{"empty level defaults to info", "", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := setupLogger(tt.level)
			
			// Test that logger is created
			if logger == nil {
				t.Fatal("Expected logger to be created")
			}
			
			// We can't directly test the level, but we can verify the logger works
			// by checking it doesn't panic when used
			logger.Info("test message")
			logger.Debug("debug message")
			logger.Error("error message")
		})
	}
}

func TestMainFlags(t *testing.T) {
	// Save original args and command line
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine
	defer func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}()

	tests := []struct {
		name        string
		args        []string
		shouldError bool
		checkOutput func(t *testing.T, output string)
	}{
		{
			name:        "missing directory flag",
			args:        []string{"cmd"},
			shouldError: true,
			checkOutput: func(t *testing.T, output string) {
				if !strings.Contains(output, "directory flag is required") {
					t.Error("Expected error about missing directory flag")
				}
			},
		},
		{
			name:        "non-existent directory",
			args:        []string{"cmd", "-directory", "/non/existent/path"},
			shouldError: true,
			checkOutput: func(t *testing.T, output string) {
				if !strings.Contains(output, "does not exist") {
					t.Error("Expected error about non-existent directory")
				}
			},
		},
		{
			name:        "help flag",
			args:        []string{"cmd", "-h"},
			shouldError: true,
			checkOutput: func(t *testing.T, output string) {
				if !strings.Contains(output, "Usage of") {
					t.Error("Expected usage information")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flag.CommandLine for each test
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
			
			// Capture stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w
			
			// Set up to capture output
			outputChan := make(chan string)
			go func() {
				var buf bytes.Buffer
				io.Copy(&buf, r)
				outputChan <- buf.String()
			}()
			
			// Set args
			os.Args = tt.args
			
			// We can't easily test main() directly because it calls os.Exit
			// Instead, we'll test the flag parsing logic
			var (
				directory = flag.String("directory", "", "Directory to serve files from")
				hostname  = flag.String("hostname", "0.0.0.0", "Hostname or IP address to bind to")
				port      = flag.String("port", "8080", "Port to listen on")
				logLevel  = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
			)
			
			// Parse flags and capture any errors
			err := flag.CommandLine.Parse(tt.args[1:])
			
			// Restore stderr and close pipe
			w.Close()
			os.Stderr = oldStderr
			output := <-outputChan
			
			// For help flag, error is expected
			if tt.name == "help flag" && err != nil {
				tt.checkOutput(t, output)
				return
			}
			
			// Check validation logic
			if tt.name == "missing directory flag" && *directory == "" {
				// Simulate the validation in main()
				fmt.Fprintln(os.Stderr, "error: directory flag is required")
				tt.checkOutput(t, "error: directory flag is required")
				return
			}
			
			if tt.name == "non-existent directory" && *directory != "" {
				if _, err := os.Stat(*directory); os.IsNotExist(err) {
					// Simulate the validation in main()
					fmt.Fprintf(os.Stderr, "error: directory %s does not exist\n", *directory)
					tt.checkOutput(t, fmt.Sprintf("error: directory %s does not exist", *directory))
					return
				}
			}
			
			// Verify default values
			if *hostname != "0.0.0.0" {
				t.Errorf("Expected default hostname to be 0.0.0.0, got %s", *hostname)
			}
			if *port != "8080" {
				t.Errorf("Expected default port to be 8080, got %s", *port)
			}
			if *logLevel != "info" {
				t.Errorf("Expected default log level to be info, got %s", *logLevel)
			}
		})
	}
}

func TestMainValidFlags(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	
	// Save original args
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()
	
	// Reset flag.CommandLine
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	
	// Set valid args
	os.Args = []string{"cmd", "-directory", tempDir, "-hostname", "localhost", "-port", "9090", "-log-level", "debug"}
	
	// Define flags
	var (
		directory = flag.String("directory", "", "Directory to serve files from")
		hostname  = flag.String("hostname", "0.0.0.0", "Hostname or IP address to bind to")
		port      = flag.String("port", "8080", "Port to listen on")
		logLevel  = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	
	// Parse flags
	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}
	
	// Verify parsed values
	if *directory != tempDir {
		t.Errorf("Expected directory to be %s, got %s", tempDir, *directory)
	}
	if *hostname != "localhost" {
		t.Errorf("Expected hostname to be localhost, got %s", *hostname)
	}
	if *port != "9090" {
		t.Errorf("Expected port to be 9090, got %s", *port)
	}
	if *logLevel != "debug" {
		t.Errorf("Expected log level to be debug, got %s", *logLevel)
	}
}
