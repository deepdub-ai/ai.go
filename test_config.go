package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ModelConfig struct {
	ModelID  string `json:"ModelID,omitempty"`
	Region   string `json:"Region,omitempty"`
	Profile  string `json:"Profile,omitempty"`
	Endpoint string `json:"Endpoint,omitempty"`
}

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error getting home directory: %v\n", err)
		return
	}

	configPath := filepath.Join(homeDir, ".ai", "model.cfg")
	fmt.Printf("Looking for config file at: %s\n", configPath)

	_, err = os.Stat(configPath)
	if os.IsNotExist(err) {
		fmt.Println("Config file does not exist!")
		return
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		return
	}

	fmt.Printf("Read config file (%d bytes)\n", len(data))

	// Replace any newlines in the file
	content := string(data)
	content = strings.ReplaceAll(content, "\n", " ")

	fmt.Printf("Content: %s\n", content)

	var config ModelConfig
	if err := json.Unmarshal([]byte(content), &config); err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		return
	}

	fmt.Println("Parsed config:")
	fmt.Printf("  ModelID: %s\n", config.ModelID)
	if config.Region != "" {
		fmt.Printf("  Region: %s\n", config.Region)
	}
	if config.Profile != "" {
		fmt.Printf("  Profile: %s\n", config.Profile)
	}
	if config.Endpoint != "" {
		fmt.Printf("  Endpoint: %s\n", config.Endpoint)
	}
}
