#!/bin/bash

set -e

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed or not in PATH"
    echo "Please install Go from https://golang.org/doc/install"
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
MAJOR=$(echo $GO_VERSION | cut -d. -f1)
MINOR=$(echo $GO_VERSION | cut -d. -f2)

if [ "$MAJOR" -lt 1 ] || ([ "$MAJOR" -eq 1 ] && [ "$MINOR" -lt 16 ]); then
    echo "Error: Go version 1.16 or higher is required"
    echo "Current version: $GO_VERSION"
    exit 1
fi

# Check if AWS credentials are configured
if [ ! -f ~/.aws/credentials ] && [ -z "$AWS_ACCESS_KEY_ID" ] && [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
    echo "Warning: AWS credentials not found"
    echo "Please configure your AWS credentials before using the tool"
    echo "You can use 'aws configure' command or set environment variables"
fi

# Clean previous builds
go clean -cache
# Build the application
echo "Building ai.go..."
go build -o ai ./cmd/ai

# Create installation directory if it doesn't exist
INSTALL_DIR="/usr/local/bin"
if [ ! -d "$INSTALL_DIR" ]; then
    echo "Creating installation directory: $INSTALL_DIR"
    sudo mkdir -p "$INSTALL_DIR"
fi

# Install the binary
echo "Installing ai to $INSTALL_DIR/ai"
sudo cp ai "$INSTALL_DIR/"

# Create symbolic link for 'ask' command (suggestion only mode)
echo "Creating 'ask' alias for suggestion-only mode"
sudo ln -sf "$INSTALL_DIR/ai" "$INSTALL_DIR/ask"

# Create the .ai directory
AI_DIR="$HOME/.ai"
if [ ! -d "$AI_DIR" ]; then
    echo "Creating AI directory: $AI_DIR"
    mkdir -p "$AI_DIR"
fi

# Migrate from old log directory if it exists
OLD_LOG_DIR="$HOME/.ai_history"
if [ -d "$OLD_LOG_DIR" ] && [ -f "$OLD_LOG_DIR/action.log" ]; then
    echo "Migrating existing logs from $OLD_LOG_DIR to $AI_DIR"
    cp "$OLD_LOG_DIR/action.log" "$AI_DIR/action.log" 2>/dev/null || true
    echo "You can safely remove the old directory: $OLD_LOG_DIR"
fi

echo "Installation complete!"
echo "You can now use the tool by running: ai \"your command\""
echo "To get command suggestions without executing them, use: ask \"your command\"" 
