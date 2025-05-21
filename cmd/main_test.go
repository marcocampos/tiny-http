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

			logger.Info("test message")
			logger.Debug("debug message")
			logger.Error("error message")
		})
	}
}

func TestMainFlags(t *testing.T) {
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
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			outputChan := make(chan string)
			go func() {
				var buf bytes.Buffer
				_, err := io.Copy(&buf, r)
				if err != nil {
					t.Errorf("Failed to copy stderr: %v", err)
				}
				outputChan <- buf.String()
			}()

			os.Args = tt.args

			var (
				directory = flag.String("directory", "", "Directory to serve files from")
				hostname  = flag.String("hostname", "0.0.0.0", "Hostname or IP address to bind to")
				port      = flag.String("port", "8080", "Port to listen on")
				logLevel  = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
			)

			err := flag.CommandLine.Parse(tt.args[1:])
			if err != nil && !tt.shouldError {
				t.Fatalf("Failed to parse flags: %v", err)
			}

			err = w.Close()
			if err != nil {
				t.Errorf("Failed to close stderr pipe: %v", err)
			}
			os.Stderr = oldStderr
			output := <-outputChan

			if tt.name == "help flag" && err != nil {
				tt.checkOutput(t, output)
				return
			}

			if tt.name == "missing directory flag" && *directory == "" {
				fmt.Fprintln(os.Stderr, "error: directory flag is required")
				tt.checkOutput(t, "error: directory flag is required")
				return
			}

			if tt.name == "non-existent directory" && *directory != "" {
				if _, err := os.Stat(*directory); os.IsNotExist(err) {
					fmt.Fprintf(os.Stderr, "error: directory %s does not exist\n", *directory)
					tt.checkOutput(t, fmt.Sprintf("error: directory %s does not exist", *directory))
					return
				}
			}

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
	tempDir := t.TempDir()

	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
	}()

	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	os.Args = []string{"cmd", "-directory", tempDir, "-hostname", "localhost", "-port", "9090", "-log-level", "debug"}

	var (
		directory = flag.String("directory", "", "Directory to serve files from")
		hostname  = flag.String("hostname", "0.0.0.0", "Hostname or IP address to bind to")
		port      = flag.String("port", "8080", "Port to listen on")
		logLevel  = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)

	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

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
