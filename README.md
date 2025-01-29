# Voice Chat Application

This is a Go implementation of a voice chat application that uses:
- PortAudio for audio capture
- OpenAI GPT-3.5 for chat responses
- (TODO) Whisper for speech-to-text
- (TODO) ElevenLabs for text-to-speech

## Prerequisites

- Go 1.21 or later
- PortAudio development libraries

### Installing PortAudio

#### macOS
```bash
brew install portaudio
```

#### Linux (Ubuntu/Debian)
```bash
sudo apt-get install portaudio19-dev
```

#### Windows
Download and install PortAudio from http://www.portaudio.com/

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd voice-chat
```

2. Install Go dependencies:
```bash
go mod download
```

## Configuration

Set your OpenAI API key as an environment variable:
```bash
export OPENAI_API_KEY="your-api-key"
```

## Running the Application

```bash
go run main.go
```

## Usage

1. Run the application
2. Start speaking when prompted
3. The application will detect voice activity and start recording
4. When you stop speaking, it will process your audio and generate a response
5. The response will be displayed (and in future versions, spoken through ElevenLabs)

## TODO

- Implement Whisper integration for speech-to-text
- Implement ElevenLabs integration for text-to-speech
- Add configuration file support
- Add error recovery and retry mechanisms
- Add voice activity detection parameters configuration

## License

MIT# ai-voice-assistant
