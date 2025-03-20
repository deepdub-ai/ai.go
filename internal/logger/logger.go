package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ANSI color codes
const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorReset  = "\033[0m"

	// Maximum history length in bytes to return (approximately 5KB)
	maxHistoryBytes = 5 * 1024
	// Maximum number of lines to return
	maxHistoryLines = 50
)

// Logger handles logging operations
type Logger struct {
	logFile    *os.File
	fileWriter io.Writer
	console    io.Writer
	logHistory bool
	mutex      sync.Mutex // Protect concurrent writes
	logPath    string     // Path to the log file
}

// New creates a new logger
func New() (*Logger, error) {
	// Ensure the log directory exists
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	logDir := filepath.Join(homeDir, ".ai")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// Set the log file path
	logPath := filepath.Join(logDir, "action.log")

	// Open the log file
	logFile, err := os.OpenFile(
		logPath,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return &Logger{
		logFile:    logFile,
		fileWriter: logFile,
		console:    os.Stdout,
		logHistory: true,
		mutex:      sync.Mutex{},
		logPath:    logPath,
	}, nil
}

// LogCommand logs a command with a timestamp
func (l *Logger) LogCommand(cmd string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Log to file without colors
	fmt.Fprintf(l.fileWriter, "\n[%s] Command: %s\n", timestamp, cmd)

	// Log to console with colors
	//fmt.Fprintf(l.console, "\n[%s] Command: %s%s%s\n", timestamp, colorRed, cmd, colorReset)
}

// LogOutput logs command output
func (l *Logger) LogOutput(output string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Write directly to the log file without timestamp to preserve output formatting
	if l.logHistory && l.logFile != nil {
		fmt.Fprint(l.fileWriter, output)
	}
}

// LogStreamOutput logs a single line of streaming output
func (l *Logger) LogStreamOutput(line string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Write directly to the log file only to avoid duplicate output on console
	if l.logHistory && l.logFile != nil {
		fmt.Fprint(l.fileWriter, line)
	}
}

// LogInfo logs information messages
func (l *Logger) LogInfo(message string) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Log to file without colors
	fmt.Fprintf(l.fileWriter, "[%s] Info: %s\n", timestamp, message)

	// Log to console with colors
	fmt.Fprintf(l.console, "[%s] Info: %s%s%s\n", timestamp, colorBlue, message, colorReset)
}

// LogError logs error messages
func (l *Logger) LogError(err error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// Log to file without colors
	fmt.Fprintf(l.fileWriter, "[%s] Error: %s\n", timestamp, err)

	// Log to console with colors
	fmt.Fprintf(l.console, "[%s] Error: %s%s%s\n", timestamp, colorYellow, err, colorReset)
}

// GetRecentHistory retrieves recent command history from the log file
// Returns the history as a string with the most recent commands and their outputs
func (l *Logger) GetRecentHistory() (string, error) {
	// We need to read the file, so make sure we're not writing to it at the same time
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Open the log file for reading (separate from the writing file handle)
	file, err := os.Open(l.logPath)
	if err != nil {
		return "", fmt.Errorf("failed to open log file for reading: %w", err)
	}
	defer file.Close()

	// Get the file size
	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to get log file info: %w", err)
	}

	// Determine how many bytes to read from the end
	fileSize := fileInfo.Size()
	readSize := maxHistoryBytes
	if fileSize < int64(readSize) {
		readSize = int(fileSize)
	}

	// Seek to the position from where we should start reading
	startPos := fileSize - int64(readSize)
	if startPos < 0 {
		startPos = 0
	}
	_, err = file.Seek(startPos, 0)
	if err != nil {
		return "", fmt.Errorf("failed to seek in log file: %w", err)
	}

	// Read the last chunk of the file
	buffer := make([]byte, readSize)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read log file: %w", err)
	}

	// Convert to string
	content := string(buffer)

	// If we started reading in the middle of a line, remove the partial line
	if startPos > 0 {
		firstNewlineIndex := strings.Index(content, "\n")
		if firstNewlineIndex >= 0 {
			content = content[firstNewlineIndex+1:]
		}
	}

	// Limit the number of lines
	lines := strings.Split(content, "\n")
	if len(lines) > maxHistoryLines {
		lines = lines[len(lines)-maxHistoryLines:]
	}

	return strings.Join(lines, "\n"), nil
}

// Close closes the logger
func (l *Logger) Close() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}
