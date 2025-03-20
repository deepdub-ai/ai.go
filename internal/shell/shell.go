package shell

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Shell handles executing commands
type Shell struct {
	LogHandler func(cmd, output string)
}

// New creates a new Shell instance
func New(logHandler func(cmd, output string)) *Shell {
	return &Shell{
		LogHandler: logHandler,
	}
}

// ExecuteCommand executes a command and returns its output
func (s *Shell) ExecuteCommand(cmd string) (string, error) {
	// Log the command
	if s.LogHandler != nil {
		s.LogHandler(cmd, "")
	}

	// Create the command
	command := exec.Command("bash", "-c", cmd)

	// Create pipes for stdout and stderr
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Combine stdout and stderr output
	var combinedOutput bytes.Buffer

	// Process stdout in real-time
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if s.LogHandler != nil {
				s.LogHandler("", line+"\n")
			}
			combinedOutput.WriteString(line + "\n")
		}
	}()

	// Process stderr in real-time
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text()
			if s.LogHandler != nil {
				s.LogHandler("", line+"\n")
			}
			combinedOutput.WriteString(line + "\n")
		}
	}()

	// Wait for the command to complete
	err = command.Wait()

	// Get the final output
	output := combinedOutput.String()

	// Return an error if the command failed
	if err != nil {
		return output, fmt.Errorf("command failed: %w\nOutput: %s", err, output)
	}

	return output, nil
}

// StreamCommand executes a command and streams its output in real-time
func (s *Shell) StreamCommand(cmd string, outputHandler func(line string)) (string, error) {
	// Log the command
	if s.LogHandler != nil {
		s.LogHandler(cmd, "")
	}

	// Create the command
	command := exec.Command("bash", "-c", cmd)

	// Create pipes for stdout and stderr
	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	// Combine stdout and stderr output
	var combinedOutput bytes.Buffer

	// Create a WaitGroup to wait for goroutines to finish
	done := make(chan struct{}, 2)

	// Process stdout in real-time
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			outputHandler(line)
			combinedOutput.WriteString(line)
		}
		done <- struct{}{}
	}()

	// Process stderr in real-time
	go func() {
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			line := scanner.Text() + "\n"
			outputHandler(line)
			combinedOutput.WriteString(line)
		}
		done <- struct{}{}
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Wait for the command to complete
	err = command.Wait()

	// Get the final output
	output := combinedOutput.String()

	// Return an error if the command failed
	if err != nil {
		return output, fmt.Errorf("command failed: %w\nOutput: %s", err, output)
	}

	return output, nil
}

// GetCurrentDirectory returns the current working directory
func (s *Shell) GetCurrentDirectory() (string, error) {
	return os.Getwd()
}

// ListFiles lists files in the current directory (limited to maxFiles)
func (s *Shell) ListFiles(maxFiles int) ([]string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	var files []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip hidden files and directories
		if strings.HasPrefix(filepath.Base(path), ".") && path != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories
		if d.IsDir() && path != dir {
			return nil
		}

		// Get the relative path from the current directory
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return nil // Skip if we can't get relative path
		}

		// Skip the current directory
		if relPath == "." {
			return nil
		}

		files = append(files, relPath)

		// Stop if we've reached the maximum number of files
		if len(files) >= maxFiles {
			return errors.New("max files reached")
		}

		return nil
	})

	// If we stopped because we reached the max files, consider it a success
	if err != nil && err.Error() != "max files reached" {
		return files, fmt.Errorf("failed to list files: %w", err)
	}

	return files, nil
}
