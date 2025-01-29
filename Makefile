.PHONY: all build run clean deps test

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary name
BINARY_NAME=voice-chat

# Main package path
MAIN_PACKAGE=.

all: deps build

build:
	$(GOBUILD) -o $(BINARY_NAME) $(MAIN_PACKAGE)

run:
	$(GORUN) $(MAIN_PACKAGE)

clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f voice_record.wav
	rm -f response.mp3

test:
	$(GOTEST) -v ./...

deps:
	$(GOMOD) download

# Install system dependencies (requires sudo on Linux)
install-deps-linux:
	sudo apt-get update
	sudo apt-get install -y portaudio19-dev

# Install system dependencies on macOS (requires brew)
install-deps-macos:
	brew install portaudio

# Check if required environment variables are set
check-env:
	@if [ -z "$(OPENAI_API_KEY)" ]; then \
		echo "Error: OPENAI_API_KEY is not set"; \
		exit 1; \
	fi
	@if [ -z "$(ELEVENLABS_API_KEY)" ]; then \
		echo "Error: ELEVENLABS_API_KEY is not set"; \
		exit 1; \
	fi

# Run with environment check
run-with-check: check-env run

# Format code
fmt:
	$(GOCMD) fmt ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Install development tools
install-tools:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Create .env file from example if it doesn't exist
init-env:
	@if [ ! -f .env ]; then \
		if [ -f .env.example ]; then \
			cp .env.example .env; \
			echo "Created .env file from .env.example. Please update with your API keys."; \
		else \
			echo "Error: .env.example file not found"; \
			exit 1; \
		fi \
	else \
		echo ".env file already exists"; \
	fi

# Help target
help:
	@echo "Available targets:"
	@echo "  all          - Download dependencies and build"
	@echo "  build        - Build the application"
	@echo "  run          - Run the application"
	@echo "  clean        - Clean build files"
	@echo "  test         - Run tests"
	@echo "  deps         - Download Go dependencies"
	@echo "  install-deps-linux  - Install system dependencies on Linux"
	@echo "  install-deps-macos  - Install system dependencies on macOS"
	@echo "  check-env    - Check required environment variables"
	@echo "  run-with-check - Run with environment variable check"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  install-tools - Install development tools"
	@echo "  init-env     - Create .env file from .env.example" 