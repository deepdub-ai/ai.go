package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// BedrockClient handles interactions with AWS Bedrock
type BedrockClient struct {
	client *bedrockruntime.Client
	config *ModelConfig
}

// ModelID is the Claude 3.7 Sonnet model ID
const ModelID = "anthropic.claude-3-7-sonnet-20250219-v1:0"

// ModelConfig holds the configuration for the AWS client
type ModelConfig struct {
	Region   string `json:"region,omitempty"`
	ModelID  string `json:"modelid,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// loadModelConfig loads the model configuration from ~/.ai/model.cfg
func loadModelConfig() (*ModelConfig, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	// Ensure the .ai directory exists
	aiDir := filepath.Join(homeDir, ".ai")
	if err := os.MkdirAll(aiDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .ai directory: %w", err)
	}

	configPath := filepath.Join(aiDir, "model.cfg")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		defaultConfig := ModelConfig{
			ModelID: ModelID,
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

	var config ModelConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	//log config data.
	fmt.Printf("Config data: %+v\n", config)
	// Use default model ID if not specified
	if config.ModelID == "" {
		config.ModelID = ModelID
	}

	return &config, nil
}

// NewBedrockClient creates a new client for Bedrock
func NewBedrockClient() (*BedrockClient, error) {
	modelConfig, err := loadModelConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load model config: %w", err)
	}

	// Setup options for AWS config
	var options []func(*config.LoadOptions) error

	// Add profile if specified
	if modelConfig.Profile != "" {
		options = append(options, config.WithSharedConfigProfile(modelConfig.Profile))
	}

	// Add region if specified
	if modelConfig.Region != "" {
		options = append(options, config.WithRegion(modelConfig.Region))
	}

	// Load AWS config with any custom options
	cfg, err := config.LoadDefaultConfig(context.TODO(), options...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create client with custom endpoint if specified
	clientOptions := []func(*bedrockruntime.Options){}
	if modelConfig.Endpoint != "" {
		clientOptions = append(clientOptions, func(o *bedrockruntime.Options) {
			o.EndpointResolver = bedrockruntime.EndpointResolverFromURL(modelConfig.Endpoint)
		})
	}

	client := bedrockruntime.NewFromConfig(cfg, clientOptions...)
	return &BedrockClient{
		client: client,
		config: modelConfig,
	}, nil
}

// MessageContent represents a content item in a message
type MessageContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string           `json:"role"`
	Content []MessageContent `json:"content"`
}

// SonnetRequest represents the request to Claude Sonnet
type SonnetRequest struct {
	AnthropicVersion string    `json:"anthropic_version"`
	MaxTokens        int       `json:"max_tokens"`
	Temperature      float64   `json:"temperature"`
	System           string    `json:"system,omitempty"`
	Messages         []Message `json:"messages"`
}

// SonnetResponse represents the response from Claude Sonnet
type SonnetResponse struct {
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
func (c *BedrockClient) GetCommandSuggestion(ctx context.Context, userQuery, currentDir string, filesList []string, commandHistory string) (string, error) {
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

	request := SonnetRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		MaxTokens:        2048,
		Temperature:      0.5,
		System:           systemPrompt,
		Messages: []Message{
			{
				Role: "user",
				Content: []MessageContent{
					{Type: "text", Text: userQuery},
				},
			},
		},
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	response, err := c.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(c.config.ModelID),
		ContentType: aws.String("application/json"),
		Body:        requestBytes,
	})
	if err != nil {
		return "", fmt.Errorf("failed to invoke model: %w", err)
	}

	var sonnetResponse SonnetResponse
	if err := json.Unmarshal(response.Body, &sonnetResponse); err != nil {
		return "", fmt.Errorf("failed to parse model response: %w", err)
	}

	// Extract the text from the response
	if len(sonnetResponse.Content) == 0 {
		return "", errors.New("empty response from model")
	}

	var responseText string
	for _, content := range sonnetResponse.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	return responseText, nil
}
