# AI.go - Shell Assistant

AI.go is a Golang-based shell assistant that uses Claude 3.7 Sonnet model to interpret natural language commands and execute shell commands on your behalf. It can connect to Claude via either AWS Bedrock or directly via the Anthropic API.

## Features

- Translate natural language requests into shell commands
- Execute commands on behalf of the user
- Safety checks for potentially dangerous commands
- Log all commands and outputs to console and file
- Back-and-forth interaction for gathering more information
- Command suggestion mode without execution ("ask" command)
- Colorized terminal output for better readability
- Command history context for smarter suggestions
- Support for both AWS Bedrock and direct Anthropic API

## Prerequisites

- Go 1.16 or later
- One of the following:
  - AWS account with access to Bedrock (and AWS credentials configured)
  - Anthropic API key

## Installation

1. Clone this repository:
```
git clone https://github.com/nir/ai.go.git
cd ai.go
```

2. Build the application:
```
go build -o ai ./cmd/ai
```

3. Install the binary to your PATH:
```
cp ai /usr/local/bin/
```

4. Create the 'ask' command alias:
```
ln -sf /usr/local/bin/ai /usr/local/bin/ask
```

Alternatively, you can run the included installation script:
```
./install.sh
```

## Configuration

You can use AI.go with either AWS Bedrock or the direct Anthropic API:

### Option 1: AWS Bedrock

The application can use your AWS credentials to access AWS Bedrock services.

You can configure your AWS credentials in multiple ways:
- Use AWS CLI: `aws configure`
- Set environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and `AWS_REGION`
- Create `~/.aws/credentials` file manually

#### AWS Bedrock Configuration

The AWS Bedrock client is configured using the `~/.ai/model.cfg` file, which is automatically created on first run with default settings. You can customize the following settings:

```json
{
  "region": "us-east-1",
  "model_id": "arn:aws:bedrock:us-east-1:AWS_ACCOUNT_ID:inference-profile/us.anthropic.claude-3-7-sonnet-20250219-v1:0",
  "profile": "default",
  "endpoint": ""
}
```

- `region`: AWS region to use (optional)
- `model_id`: Bedrock model ID (defaults to Claude 3.7 Sonnet)
- `profile`: AWS profile to use (optional)
- `endpoint`: Custom endpoint URL (optional)

**Note:** You will need to add your AWS Bedrock client ID to the model configuration before using the application.

### Option 2: Direct Anthropic API

You can also use AI.go directly with the Anthropic API. This option requires an Anthropic API key, which you can get from [Anthropic's website](https://console.anthropic.com/).

You can set your Anthropic API key in one of two ways:
1. Set the `ANTHROPIC_API_KEY` environment variable:
   ```
   export ANTHROPIC_API_KEY=your_api_key
   ```
2. Configure it in the `~/.ai/anthropic.cfg` file:
   ```json
   {
     "api_key": "your_api_key",
     "model_id": "claude-3-7-sonnet-20250219"
   }
   ```

- `api_key`: Your Anthropic API key
- `model_id`: Anthropic model ID (defaults to Claude 3.7 Sonnet)

The application will automatically use the Anthropic API if a valid API key is found, otherwise it will fall back to AWS Bedrock.

## Usage

### Execute Commands

Run the assistant by providing a natural language description of what you want to do:

```
ai "list all large files in the current directory"
```

The assistant will:
1. Ask Claude 3.7 Sonnet for the appropriate command
2. Show you the suggested command
3. Execute the command if it's safe (or ask for approval if it's not)
4. Display the output

### Suggestion-Only Mode

If you want to get command suggestions without executing them, use the `ask` command:

```
ask "create a backup of the important files"
```

This will show you what command Claude would suggest without running it. This is useful for:
- Learning which commands to use for specific tasks
- Checking commands before execution for complex or potentially dangerous operations
- Understanding how to perform tasks manually

## Logs

All commands and outputs are logged to:
- The console (real-time)
- `~/.ai/action.log` (persistent)

This history is also used to provide context for multi-step operations, making Claude's suggestions more accurate.

## Examples

```
$ ai "find all Go files that contain the word 'error'"
$ ai "create a zip backup of the current directory"
$ ai "clean up temporary files older than 30 days"
$ ask "how would I set up a cron job to run daily backups"
```

## Safety

The assistant will ask for your approval before executing commands it considers potentially unsafe. Examples of unsafe commands include:
- Commands that modify or delete files
- Commands that affect system configuration
- Commands with wildcards that could potentially affect many files

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details. 
