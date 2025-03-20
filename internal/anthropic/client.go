package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModelID is the Claude 3.7 Sonnet model ID
const ModelID = "claude-3-7-sonnet-20250219"

// ClientConfig holds the configuration for the Anthropic client
type ClientConfig struct {
	APIKey  string `json:"api_key,omitempty"`
	ModelID string `json:"model_id,omitempty"`
}

// AnthropicClient handles interactions with Anthropic API
type AnthropicClient struct {
	config *ClientConfig
}

// MessageContent represents a content item in a message
type MessageContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string           `json:"role"`
	Content []MessageContent `json:"content,omitempty"`
}

// AnthropicRequest represents the request to Claude
type AnthropicRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	System      string    `json:"system,omitempty"`
	Messages    []Message `json:"messages"`
}

// AnthropicResponse represents the response from Claude
type AnthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
}

// Command represents the parsed command response from the model
type Command struct {
	Safe        bool   `json:"safe"`
	Command     string `json:"command"`
	Reason      string `json:"reason"`
	IsFinal     bool   `json:"is_final"`
	NeedsOutput bool   `json:"needs_output"`
}

// loadClientConfig loads the client configuration from ~/.ai/anthropic.cfg
func loadClientConfig() (*ClientConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Ensure the .ai directory exists
	aiDir := filepath.Join(homeDir, ".ai")
	if err := os.MkdirAll(aiDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .ai directory: %w", err)
	}

	configPath := filepath.Join(aiDir, "anthropic.cfg")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		defaultConfig := ClientConfig{
			ModelID: ModelID,
			APIKey:  "",
		}

		configData, err := json.MarshalIndent(defaultConfig, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal default config: %w", err)
		}

		if err := os.WriteFile(configPath, configData, 0644); err != nil {
			return nil, fmt.Errorf("failed to write default config file: %w", err)
		}

		return &defaultConfig, nil
	}

	// Read existing config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ClientConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Use default model ID if not specified
	if config.ModelID == "" {
		config.ModelID = ModelID
	}

	// Check for API key in environment if not in config
	if config.APIKey == "" {
		config.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}

	return &config, nil
}

// NewAnthropicClient creates a new client for Anthropic API
func NewAnthropicClient() (*AnthropicClient, error) {
	clientConfig, err := loadClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load client config: %w", err)
	}

	// Validate API key
	if clientConfig.APIKey == "" {
		return nil, errors.New("Anthropic API key not found in config or environment variable ANTHROPIC_API_KEY")
	}

	return &AnthropicClient{
		config: clientConfig,
	}, nil
}

// ParseCommandResponse parses the model's response into a command structure
func ParseCommandResponse(responseText string) (*Command, error) {
	// Check if the response is wrapped in markdown code block
	jsonText := responseText

	// Strip markdown code block formatting if present
	markdownStart := "```json"
	markdownEnd := "```"
	if strings.Contains(jsonText, markdownStart) {
		startIndex := strings.Index(jsonText, markdownStart) + len(markdownStart)
		endIndex := strings.LastIndex(jsonText, markdownEnd)
		if endIndex > startIndex {
			jsonText = jsonText[startIndex:endIndex]
		}
	}

	// Trim any leading/trailing whitespace
	jsonText = strings.TrimSpace(jsonText)

	var cmd Command
	if err := json.Unmarshal([]byte(jsonText), &cmd); err != nil {
		return nil, fmt.Errorf("failed to parse command response: %w", err)
	}
	return &cmd, nil
}

// GetCommandSuggestion asks the model for command suggestions
func (c *AnthropicClient) GetCommandSuggestion(ctx context.Context, userQuery, currentDir string, filesList []string, commandHistory string) (string, error) {
	// Create system prompt with history if provided
	var systemPrompt string
	if commandHistory != "" {
		systemPrompt = fmt.Sprintf(
			"You are an AI assistant providing shell commands to execute tasks. Your job is to translate user requests into the exact commands needed.\n"+
				"Current directory: %s\n"+
				"Files in directory (limited to 1000): %v\n\n"+
				"Recent command history (for context):\n%s\n\n"+
				"Provide the exact command or commands to run in response to the user's request. "+
				"Format your response as JSON with these fields:\n"+
				"- 'safe': a boolean indicating if the command is safe to run automatically\n"+
				"- 'command': the exact command(s) to run\n"+
				"- 'reason': a brief explanation of what the command does\n"+
				"- 'is_final': a boolean indicating if this is the final command to complete the user's request (true) or if more commands will be needed (false)\n"+
				"- 'needs_output': a boolean indicating if you need to see the output of this command to determine the next step\n\n"+
				"If you need more information, respond with JSON where 'needs_output' is true and the 'command' field contains the command needed to gather that information. "+
				"The output of this command will be shown to you.\n\n"+
				"IMPORTANT: Return ONLY the raw JSON data without any markdown formatting like ```json or ```. Just the plain JSON object.",
			currentDir, filesList, commandHistory)
	} else {
		systemPrompt = fmt.Sprintf(
			"You are an AI assistant providing shell commands to execute tasks. Your job is to translate user requests into the exact commands needed.\n"+
				"Current directory: %s\n"+
				"Files in directory (limited to 1000): %v\n\n"+
				"Provide the exact command or commands to run in response to the user's request. "+
				"Format your response as JSON with these fields:\n"+
				"- 'safe': a boolean indicating if the command is safe to run automatically\n"+
				"- 'command': the exact command(s) to run\n"+
				"- 'reason': a brief explanation of what the command does\n"+
				"- 'is_final': a boolean indicating if this is the final command to complete the user's request (true) or if more commands will be needed (false)\n"+
				"- 'needs_output': a boolean indicating if you need to see the output of this command to determine the next step\n\n"+
				"If you need more information, respond with JSON where 'needs_output' is true and the 'command' field contains the command needed to gather that information. "+
				"The output of this command will be shown to you.\n\n"+
				"IMPORTANT: Return ONLY the raw JSON data without any markdown formatting like ```json or ```. Just the plain JSON object.",
			currentDir, filesList)
	}

	request := AnthropicRequest{
		Model:       c.config.ModelID,
		MaxTokens:   2048,
		Temperature: 0.5,
		System:      systemPrompt,
		Messages: []Message{
			{
				Role: "user",
				Content: []MessageContent{
					{Type: "text", Text: userQuery},
				},
			},
		},
	}

	// Convert request to JSON
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// We'll implement the HTTP request in a separate function
	responseText, err := c.sendRequest(ctx, requestBytes)
	if err != nil {
		return "", err
	}

	return responseText, nil
}

// sendRequest sends the request to the Anthropic API
func (c *AnthropicClient) sendRequest(ctx context.Context, requestBody []byte) (string, error) {
	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: time.Second * 120, // 2 minute timeout
	}

	// Create request
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.anthropic.com/v1/messages",
		strings.NewReader(string(requestBody)),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Send request
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response AnthropicResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("failed to parse API response: %w", err)
	}

	// Extract the text from the response
	if len(response.Content) == 0 {
		return "", errors.New("empty response from model")
	}

	var responseText string
	for _, content := range response.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return responseText, nil
}
