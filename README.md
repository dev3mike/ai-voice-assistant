# AI Voice Assistant 🎙️ 🤖

**⚠️ Experimental Project**

This is an experimental voice assistant that can have natural conversations with you! It listens to your voice, understands what you say, and responds back with a human-like voice.

## How it Works 🔍

1. Records your voice when you speak 🎤
2. Converts your speech to text using OpenAI Whisper
3. Generates a response using GPT-4
4. Converts the response to speech using ElevenLabs
5. Plays the response through your speakers 🔊

## Requirements 📋

### API Keys 🔑
You need to get these API keys:
- OpenAI API key (for GPT-4 and Whisper)
- ElevenLabs API key (for voice generation)

### System Dependencies 🖥️
Before running the project, make sure you have these installed:

#### macOS:
```bash
brew install portaudio
```

#### Linux:
```bash
sudo apt-get install portaudio19-dev
```

#### Windows:
PortAudio should work out of the box with the Go package

### Go Dependencies 📦
The project uses these main Go packages:
- github.com/gordonklaus/portaudio (for audio recording)
- github.com/go-audio/wav (for WAV file handling)
- github.com/sashabaranov/go-openai (for OpenAI API)
- github.com/haguro/elevenlabs-go (for ElevenLabs API)

## Setup 🚀

1. Clone the repository
2. Create a `.env` file with your API keys:
```env
OPENAI_API_KEY=your_openai_key_here
ELEVENLABS_API_KEY=your_elevenlabs_key_here
ELEVENLABS_VOICE_ID=optional_voice_id_here
```
3. Run the project:
```bash
go run main.go
```

## Features ✨

- Voice activity detection
- Background noise adaptation
- Natural conversation flow
- Automatic conversation ending detection
- Debug mode for troubleshooting

## Limitations ⚠️

Since this is an experimental project:
- May have occasional recognition issues
- Voice quality depends on ElevenLabs model
- Requires good microphone input
- Network delays can affect response time

## Contributing 🤝

Feel free to experiment and contribute! This is a fun project to learn about:
- Audio processing
- Voice recognition
- Large Language Models
- Text-to-Speech systems

## License 📄

MIT License - Feel free to use and modify!
