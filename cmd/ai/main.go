package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nir/ai.go/internal/anthropic"
	"github.com/nir/ai.go/internal/aws"
	"github.com/nir/ai.go/internal/logger"
	"github.com/nir/ai.go/internal/shell"
)

const (
	maxFiles = 1000

	// ANSI color codes
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorReset  = "\033[0m"
)

// Model represents the application state
type Model struct {
	spinner  spinner.Model
	response string
	err      error
	done     bool
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles model updates
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	case error:
		m.err = msg
		m.done = true
		return m, tea.Quit
	case string:
		m.response = msg
		m.done = true
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the current state
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\nError: %v\n", m.err)
	}
	if m.done {
		return ""
	}
	return fmt.Sprintf("\n %s Thinking...\n", m.spinner.View())
}

// ClientType determines which client to use (AWS Bedrock or direct Anthropic API)
type ClientType int

const (
	// ClientTypeAWS uses AWS Bedrock
	ClientTypeAWS ClientType = iota
	// ClientTypeAnthropic uses direct Anthropic API
	ClientTypeAnthropic
)

// Client interface defines methods that both clients must implement
type Client interface {
	GetCommandSuggestion(ctx context.Context, userQuery, currentDir string, filesList []string, commandHistory string) (string, error)
}

// waitWithSpinner runs a spinner while waiting for Claude's response
func waitWithSpinner(ctx context.Context, client Client, query, currentDir string, files []string, commandHistory string) (string, error) {
	// Initialize spinner model
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Create initial model
	m := Model{
		spinner: s,
	}

	// Create a channel for the response
	responseChan := make(chan string)
	errChan := make(chan error)
	done := make(chan struct{})

	// Run the API call in a goroutine
	go func() {
		response, err := client.GetCommandSuggestion(ctx, query, currentDir, files, commandHistory)
		if err != nil {
			errChan <- err
		} else {
			responseChan <- response
		}
		close(done)
	}()

	// Create bubbletea program without alternate screen to avoid terminal state issues
	p := tea.NewProgram(m)

	// Start the program
	go func() {
		if _, err := p.Run(); err != nil {
			errChan <- err
		}
	}()

	// Wait for either a response or an error
	var result string
	var resultErr error

	select {
	case response := <-responseChan:
		result = response
	case err := <-errChan:
		resultErr = err
	case <-done:
		// Check if we have a response
		select {
		case response := <-responseChan:
			result = response
		case err := <-errChan:
			resultErr = err
		default:
			resultErr = fmt.Errorf("no response received")
		}
	}

	// Ensure program is properly quit
	p.Quit()

	// Reset terminal state using ANSI escape codes
	fmt.Print("\033[?25h") // Show cursor
	fmt.Print("\033[0m")   // Reset all attributes
	fmt.Println()          // Print newline for clean spacing

	// Reset the terminal using stty
	sh := shell.New(nil)
	sh.StreamCommand("stty sane", func(line string) {})

	if resultErr != nil {
		return "", resultErr
	}
	return result, nil
}

// getClient initializes the appropriate client based on the config
func getClient(log *logger.Logger) (Client, error) {
	// Check if API key is set directly, use Anthropic client if it is
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey != "" {
		// If ANTHROPIC_API_KEY environment variable is set, try to use the Anthropic client
		anthropicClient, err := anthropic.NewAnthropicClient()
		if err == nil {
			log.LogInfo("Using Anthropic API client (from environment variable)")
			return anthropicClient, nil
		}
		// If there was an error initializing the Anthropic client, log it and try AWS
		log.LogError(fmt.Errorf("failed to initialize Anthropic client with env var: %w", err))
	}

	// Check if Anthropic API key exists in config
	homeDir, err := os.UserHomeDir()
	if err == nil {
		configPath := filepath.Join(homeDir, ".ai", "anthropic.cfg")
		if _, err := os.Stat(configPath); err == nil {
			// Config exists, try to use the Anthropic client
			anthropicClient, err := anthropic.NewAnthropicClient()
			if err == nil {
				log.LogInfo("Using Anthropic API client (from config file)")
				return anthropicClient, nil
			}
			// If there was an error initializing the Anthropic client, log it and try AWS
			log.LogError(fmt.Errorf("failed to initialize Anthropic client with config: %w", err))
		}
	}

	// Otherwise, use AWS client
	awsClient, err := aws.NewBedrockClient()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AWS client: %w", err)
	}

	log.LogInfo("Using AWS Bedrock client")
	return awsClient, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ai \"what you want to do\"")
		os.Exit(1)
	}

	// Check if we're running in "ask" mode (suggestion only, no execution)
	executableName := filepath.Base(os.Args[0])
	askModeOnly := executableName == "ask"

	// Combine all arguments as the user query
	userQuery := strings.Join(os.Args[1:], " ")

	// Initialize logger
	log, err := logger.New()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	// Initialize shell
	sh := shell.New(func(cmd, output string) {
		if cmd != "" {
			log.LogCommand(cmd)
		}
		if output != "" {
			log.LogStreamOutput(output)
		}
	})

	// Get current directory
	currentDir, err := sh.GetCurrentDirectory()
	if err != nil {
		log.LogError(fmt.Errorf("failed to get current directory: %w", err))
		os.Exit(1)
	}

	// List files in the current directory
	files, err := sh.ListFiles(maxFiles)
	if err != nil {
		log.LogError(fmt.Errorf("failed to list files: %w", err))
		os.Exit(1)
	}

	// Initialize client
	client, err := getClient(log)
	if err != nil {
		log.LogError(fmt.Errorf("failed to initialize AI client: %w", err))
		os.Exit(1)
	}

	// Create a context with a timeout
	ctx := context.Background()

	// Log the user query
	if askModeOnly {
		log.LogInfo(fmt.Sprintf("Ask Mode: %s", userQuery))
	} else {
		log.LogInfo(fmt.Sprintf("User Query: %s", userQuery))
	}

	// Process user query in a loop to handle back-and-forth interactions
	commandCount := 0
	for {
		commandCount++

		// Get command suggestion from Sonnet
		log.LogInfo("Asking Claude for command suggestion...")
		if commandCount > 1 {
			fmt.Println("\n--- Asking Claude for next command... ---\n")
		}

		// Fetch recent command history for context
		var commandHistory string
		history, histErr := log.GetRecentHistory()
		if histErr != nil {
			log.LogError(fmt.Errorf("failed to get command history: %w", histErr))
			// Continue without history if we can't get it
		} else {
			commandHistory = history
			log.LogInfo(fmt.Sprintf("Including %d bytes of command history for context", len(commandHistory)))
		}

		// Get command suggestion with spinner
		modelResponse, err := waitWithSpinner(ctx, client, userQuery, currentDir, files, commandHistory)
		if err != nil {
			log.LogError(fmt.Errorf("failed to get command suggestion: %w", err))
			os.Exit(1)
		}

		// Parse the model response
		cmd, err := aws.ParseCommandResponse(modelResponse)
		if err != nil {
			log.LogError(fmt.Errorf("failed to parse model response: %s\nError: %v", modelResponse, err))
			fmt.Println("Raw model response:", modelResponse)
			os.Exit(1)
		}

		// Log the command suggestion
		log.LogInfo(fmt.Sprintf("Suggested Command: %s", cmd.Command))
		log.LogInfo(fmt.Sprintf("Reason: %s", cmd.Reason))
		log.LogInfo(fmt.Sprintf("Safe: %t", cmd.Safe))
		log.LogInfo(fmt.Sprintf("Is Final: %t", cmd.IsFinal))
		log.LogInfo(fmt.Sprintf("Needs Output: %t", cmd.NeedsOutput))

		// Display the command suggestion
		if askModeOnly {
			fmt.Printf("\n%s💡 Suggested Command:%s\n", colorGreen, colorReset)
			fmt.Printf("%s%s%s\n\n", colorRed, cmd.Command, colorReset)
			fmt.Printf("Reason: %s\n", cmd.Reason)
			fmt.Printf("Safety: %s\n", getSafetyText(cmd.Safe))

			if !cmd.IsFinal {
				if cmd.NeedsOutput {
					fmt.Printf("\n%s🔄 This is an intermediate command. Claude would need to see its output to determine next steps.%s\n", colorBlue, colorReset)
				} else {
					fmt.Printf("\n%s🔄 This is part of a multi-step process. More commands would follow.%s\n", colorBlue, colorReset)
				}
			} else {
				fmt.Printf("\n%s✅ This is the final command to complete your request.%s\n", colorGreen, colorReset)
			}

			// In ask mode, we're done after the first command suggestion
			break
		}

		// Inform the user about the nature of the command
		if !cmd.IsFinal {
			if cmd.NeedsOutput {
				fmt.Printf("\n%s🔄 This is an intermediate command. Claude needs to see its output to determine next steps.%s\n", colorBlue, colorReset)
			} else {
				fmt.Printf("\n%s🔄 This is part of a multi-step process. More commands will follow.%s\n", colorBlue, colorReset)
			}
		} else {
			fmt.Printf("\n%s✅ This is the final command to complete your request.%s\n", colorGreen, colorReset)
		}

		// Check if the command is safe
		if !cmd.Safe {
			fmt.Printf("%s⚠️  Caution: The command is marked as not safe. ⚠️%s\n", colorYellow, colorReset)
			fmt.Printf("Command: %s%s%s\n", colorRed, cmd.Command, colorReset)
			fmt.Printf("Reason: %s\n", cmd.Reason)
			fmt.Print("Do you want to run this command anyway? (y/n): ")

			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			answer := strings.ToLower(scanner.Text())

			if answer != "y" && answer != "yes" {
				fmt.Println("Command execution cancelled by user.")
				return
			}
		}

		// Execute the command with streaming output
		fmt.Printf("\n🔄 Executing command: %s%s%s\n", colorRed, cmd.Command, colorReset)
		fmt.Println("-------------------------------------------------------------------------")

		var output string
		var execErr error

		// Use the streaming command execution
		output, execErr = sh.StreamCommand(cmd.Command, func(line string) {
			// This function is called for each line of output as it's produced
			// We don't need to do anything here since the LogHandler in the shell will log it
			fmt.Print(line) // Print directly to console for immediate feedback
		})

		fmt.Println("-------------------------------------------------------------------------")

		if execErr != nil {
			log.LogError(fmt.Errorf("command execution failed: %w", execErr))
			fmt.Printf("%s⚠️ Command execution error: %v%s\n", colorYellow, execErr, colorReset)
			// Don't exit on command failure, just log it
		}

		// If this is the final command or we don't need output, break the loop
		if cmd.IsFinal && !cmd.NeedsOutput {
			fmt.Printf("%s✅ Task completed successfully!%s\n", colorGreen, colorReset)
			break
		}

		// If the command needs output for next steps, update the user query
		if cmd.NeedsOutput {
			userQuery = fmt.Sprintf("I ran the command '%s' and got the output:\n%s\nPlease provide the next command to continue with my original request: %s",
				cmd.Command, output, userQuery)
		} else {
			// Just continue with the next command in sequence
			userQuery = fmt.Sprintf("I successfully ran '%s'. What's the next command to continue with my original request: %s",
				cmd.Command, userQuery)
		}
	}
}

// getSafetyText returns a colored text representation of the safety status
func getSafetyText(safe bool) string {
	if safe {
		return colorGreen + "Safe to run automatically" + colorReset
	}
	return colorYellow + "Requires approval (potentially unsafe)" + colorReset
}
