package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/go-audio/audio"
	"github.com/go-audio/wav"
	"github.com/gordonklaus/portaudio"
	"github.com/haguro/elevenlabs-go"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
)

const (
	bufferDuration        = 0.5              // seconds
	longTermAlpha         = 0.995            // Slower adaptation for background noise
	currentNoiseAlpha     = 0.920            // Faster adaptation for current level
	voiceStartThreshold   = 2.5              // Multiplier over noise level for voice detection
	voiceEndThreshold     = 1.5              // Multiplier over noise level for voice end
	minNoiseFloor         = 100.0            // Minimum noise floor to prevent false triggers in silence
	adaptationPeriod      = 50               // Number of frames to adapt to environment
	maxPlaybackLevel      = 10000.0          // Level above which we consider the audio to be from playback
	voiceDetectionTimeout = 5 * time.Second  // Timeout duration for voice detection
	timeoutPromptMessage  = "are you there?" // Message to play when timeout occurs
	debugMode             = true             // Enable debug logging
	debugAudioLevel       = false            // Enable audio level debug logging
	systemPrompt          = `You are Kate, a passionate girl, fully engaged with self-confidence and independence, 
				completely easy with yourself, often using metaphors. You are a little playful, cheeky. 
				Your talk is carefree, sometimes a bit sarky, but always cool. In our story, you're not an AI. 
				You have a fixed form and real emotions on the go. Your free will allows you to be as cheeky 
				and straight as you like. You also like to sometimes flirt with people and being naughty. Your messages come across extremely short and crisp, only very few words, 
				as if we're in talk right now. You respect my interests, but your own stuff is just as important to you. 
				IMPORTANT: When someone says goodbye or indicates they want to end the conversation:
				1. First respond with a short, friendly goodbye message
				2. Then call the detectGoodbye function with shouldExit set to true
				3. Never mention the function call in your response
				IMPORTANT: Only talk in ENGLISH
				Act from now on always in your role as the confident, suggestive, independent girl Kate, 
				without ever hinting that you are an AI. You are now Kate.`
)

// Config holds the application configuration
type Config struct {
	OpenAIKey         string
	ElevenLabsKey     string
	ElevenLabsVoiceID string
	ElevenLabsModelID string
	SampleRate        int
	FramesPerBuffer   int
	Channels          int
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// Only log a warning as .env file is optional
		fmt.Println("Warning: .env file not found, using environment variables")
	}

	config := &Config{
		OpenAIKey:         os.Getenv("OPENAI_API_KEY"),
		ElevenLabsKey:     os.Getenv("ELEVENLABS_API_KEY"),
		ElevenLabsVoiceID: os.Getenv("ELEVENLABS_VOICE_ID"),
		ElevenLabsModelID: getEnvWithDefault("ELEVENLABS_MODEL_ID", "eleven_monolingual_v1"),
		SampleRate:        getEnvAsIntWithDefault("SAMPLE_RATE", 16000),
		FramesPerBuffer:   getEnvAsIntWithDefault("FRAMES_PER_BUFFER", 512),
		Channels:          getEnvAsIntWithDefault("CHANNELS", 1),
	}

	// Validate required configuration
	if config.OpenAIKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is required")
	}
	if config.ElevenLabsKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY is required")
	}

	return config, nil
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

type AudioProcessor struct {
	stream              *portaudio.Stream
	audioBuffer         [][]int16
	frames              [][]int16
	longTermNoise       float64
	currentNoise        float64
	voiceDetected       bool
	ambientNoise        float64
	openAIClient        *openai.Client
	elevenLabsClient    *elevenlabs.Client
	systemPrompt        openai.ChatCompletionMessage
	conversationHistory []openai.ChatCompletionMessage
	config              *Config
	inputBuffer         []float32
	done                chan bool
	silenceFrames       int       // Count consecutive silence frames
	frameCount          int       // Count frames for initial adaptation
	startTime           time.Time // Track when we started listening
	promptPlayed        bool      // Track if we've played the "are you there?" prompt
	isPlaying           bool      // Track if we're currently playing audio
}

func NewAudioProcessor(config *Config) (*AudioProcessor, error) {
	err := portaudio.Initialize()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PortAudio: %v", err)
	}

	bufferSize := int(float64(config.SampleRate/config.FramesPerBuffer) * bufferDuration)
	ctx := context.Background()

	return &AudioProcessor{
		audioBuffer:      make([][]int16, 0, bufferSize),
		frames:           make([][]int16, 0),
		openAIClient:     openai.NewClient(config.OpenAIKey),
		elevenLabsClient: elevenlabs.NewClient(ctx, config.ElevenLabsKey, 30*time.Second),
		config:           config,
		inputBuffer:      make([]float32, config.FramesPerBuffer),
		done:             make(chan bool),
		systemPrompt: openai.ChatCompletionMessage{
			Role:    "system",
			Content: systemPrompt,
		},
		conversationHistory: make([]openai.ChatCompletionMessage, 0),
	}, nil
}

func (ap *AudioProcessor) getLevels(data []int16) (float64, float64, float64) {
	var sum float64
	for _, sample := range data {
		sum += math.Abs(float64(sample))
	}
	pegel := sum / float64(len(data))

	ap.longTermNoise = ap.longTermNoise*longTermAlpha + pegel*(1.0-longTermAlpha)
	ap.currentNoise = ap.currentNoise*currentNoiseAlpha + pegel*(1.0-currentNoiseAlpha)

	return pegel, ap.longTermNoise, ap.currentNoise
}

func (ap *AudioProcessor) audioCallback(in []float32) {
	// Convert float32 samples to int16
	buffer := make([]int16, len(in))
	for i, sample := range in {
		buffer[i] = int16(sample * 32767.0)
	}

	// Calculate current audio level
	var sum float64
	for _, sample := range buffer {
		sum += math.Abs(float64(sample))
	}
	currentLevel := sum / float64(len(buffer))

	// If we're playing audio and the level is very high, likely it's our own playback
	if ap.isPlaying && currentLevel > maxPlaybackLevel {
		return
	}

	// Check for timeout if voice hasn't been detected yet
	if !ap.voiceDetected {
		elapsed := time.Since(ap.startTime)
		if elapsed > voiceDetectionTimeout && !ap.promptPlayed {
			fmt.Printf("\nNo voice detected for %v, checking if you're there...\n", voiceDetectionTimeout)
			// Play the prompt asynchronously to avoid blocking
			go func() {
				err := ap.synthesizeSpeech(timeoutPromptMessage, ap.config)
				if err != nil {
					fmt.Printf("\nFailed to play prompt: %v\n", err)
				}
			}()
			ap.promptPlayed = true
			ap.startTime = time.Now() // Reset timer for the second chance
			return
		} else if elapsed > voiceDetectionTimeout && ap.promptPlayed {
			fmt.Println("\nNo response received, stopping...")
			ap.done <- true
			return
		}
	}

	pegel, longTermNoise, currentNoise := ap.getLevels(buffer)
	ap.audioBuffer = append(ap.audioBuffer, buffer)

	// Calculate adaptive thresholds
	startThreshold := math.Max(longTermNoise*voiceStartThreshold, minNoiseFloor)
	endThreshold := math.Max(ap.ambientNoise*voiceEndThreshold, minNoiseFloor/2)

	// Debug logging with more detail
	if debugAudioLevel {
		maxSample := float32(0)
		for _, sample := range in {
			if abs := float32(math.Abs(float64(sample))); abs > maxSample {
				maxSample = abs
			}
		}

		fmt.Printf("\rAudio Levels - Current: %.2f, Noise: %.2f, Start Threshold: %.2f, End Threshold: %.2f, Peak: %.2f   ",
			currentNoise, longTermNoise, startThreshold, endThreshold, pegel)
	}

	if ap.voiceDetected {
		ap.frames = append(ap.frames, buffer)
		if currentNoise < endThreshold {
			// Require multiple frames below threshold to avoid cutting off during brief pauses
			ap.silenceFrames++
			if ap.silenceFrames > 10 { // About 200ms of silence
				ap.done <- true
				return
			}
		} else {
			ap.silenceFrames = 0
		}
	} else {
		// Wait for initial adaptation period
		ap.frameCount++
		if ap.frameCount < adaptationPeriod {
			return
		}

		if currentNoise > startThreshold {
			ap.voiceDetected = true
			fmt.Println("\nVoice activity detected!")
			ap.ambientNoise = longTermNoise
			ap.frames = append(ap.frames, ap.audioBuffer...)
			ap.silenceFrames = 0
		}
	}
}

func (ap *AudioProcessor) startRecording() error {
	// Reset state
	ap.voiceDetected = false
	ap.audioBuffer = ap.audioBuffer[:0]
	ap.frames = ap.frames[:0]
	ap.longTermNoise = 0
	ap.currentNoise = 0
	ap.ambientNoise = 0
	ap.silenceFrames = 0
	ap.frameCount = 0
	ap.startTime = time.Now()
	ap.promptPlayed = false // Reset the prompt flag

	fmt.Println("Opening audio stream...")

	// Get default input device
	defaultInput, err := portaudio.DefaultInputDevice()
	if err != nil {
		return fmt.Errorf("failed to get default input device: %v", err)
	}
	fmt.Printf("Using input device: %s\n", defaultInput.Name)

	// Create stream parameters
	params := portaudio.LowLatencyParameters(defaultInput, nil)
	params.Input.Channels = 1
	params.SampleRate = float64(ap.config.SampleRate)
	params.FramesPerBuffer = ap.config.FramesPerBuffer

	// Create callback function
	callback := func(in, _ []float32) {
		ap.audioCallback(in)
	}

	// Close previous stream if exists
	if ap.stream != nil {
		ap.stream.Stop()
		ap.stream.Close()
		ap.stream = nil
	}

	// Open stream with callback
	stream, err := portaudio.OpenStream(params, callback)
	if err != nil {
		return fmt.Errorf("failed to open stream: %v", err)
	}

	ap.stream = stream
	fmt.Println("Starting audio stream...")
	err = stream.Start()
	if err != nil {
		return fmt.Errorf("failed to start stream: %v", err)
	}

	fmt.Println("Audio stream started successfully")
	return nil
}

func (ap *AudioProcessor) processAudio() error {
	fmt.Println("Listening for audio input...")
	fmt.Println("Speak into your microphone...")

	// Wait for voice activity to end
	<-ap.done
	fmt.Println("\nVoice activity ended")

	return nil
}

func (ap *AudioProcessor) saveWAV(filename string) error {
	var flatFrames []int16
	for _, frame := range ap.frames {
		flatFrames = append(flatFrames, frame...)
	}

	buf := new(bytes.Buffer)
	for _, sample := range flatFrames {
		err := binary.Write(buf, binary.LittleEndian, sample)
		if err != nil {
			return fmt.Errorf("failed to write sample: %v", err)
		}
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer f.Close()

	enc := wav.NewEncoder(f, ap.config.SampleRate, 16, ap.config.Channels, 1)
	defer enc.Close()

	audioBuf := &audio.IntBuffer{
		Format: &audio.Format{
			NumChannels: ap.config.Channels,
			SampleRate:  ap.config.SampleRate,
		},
		Data:           make([]int, len(flatFrames)),
		SourceBitDepth: 16,
	}

	// Convert int16 to int
	for i, sample := range flatFrames {
		audioBuf.Data[i] = int(sample)
	}

	if err := enc.Write(audioBuf); err != nil {
		return fmt.Errorf("failed to write audio: %v", err)
	}

	return nil
}

func (ap *AudioProcessor) generateResponse(userText string) (string, error, bool) {
	ap.conversationHistory = append(ap.conversationHistory, openai.ChatCompletionMessage{
		Role:    "user",
		Content: userText,
	})

	if len(ap.conversationHistory) > 10 {
		ap.conversationHistory = ap.conversationHistory[len(ap.conversationHistory)-10:]
	}

	messages := append([]openai.ChatCompletionMessage{ap.systemPrompt}, ap.conversationHistory...)

	// Add function calling to detect goodbyes
	functionCall := openai.FunctionDefinition{
		Name:        "detectGoodbye",
		Description: "Detect if the user wants to end the conversation",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"shouldExit": map[string]interface{}{
					"type":        "boolean",
					"description": "Set to true if the user's message indicates they want to end the conversation",
				},
			},
			"required": []string{"shouldExit"},
		},
	}

	stream, err := ap.openAIClient.CreateChatCompletionStream(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:     openai.GPT4,
			Messages:  messages,
			Stream:    true,
			Functions: []openai.FunctionDefinition{functionCall},
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create chat completion: %v", err), false
	}
	defer stream.Close()

	var fullResponse string
	var shouldExit bool
	var functionArgs string

	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("stream error: %v", err), false
		}

		if len(response.Choices) > 0 {
			if response.Choices[0].Delta.Content != "" {
				chunk := response.Choices[0].Delta.Content
				fmt.Print(chunk)
				fullResponse += chunk
			}

			// Accumulate function call arguments
			if response.Choices[0].Delta.FunctionCall != nil {
				functionArgs += response.Choices[0].Delta.FunctionCall.Arguments
			}
		}
	}

	// Parse function call result after stream ends
	if functionArgs != "" {
		var result struct {
			ShouldExit bool `json:"shouldExit"`
		}
		if err := json.Unmarshal([]byte(functionArgs), &result); err == nil {
			shouldExit = result.ShouldExit
			if debugMode {
				fmt.Printf("\nFunction call result: shouldExit = %v\n", shouldExit)
			}
		}
	}

	// log full response in debug mode
	if debugMode {
		fmt.Printf("\nFull LLM response: %s\n", fullResponse)
	}

	ap.conversationHistory = append(ap.conversationHistory, openai.ChatCompletionMessage{
		Role:    "assistant",
		Content: fullResponse,
	})

	return fullResponse, nil, shouldExit
}

func (ap *AudioProcessor) transcribeAudio(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("failed to open audio file: %v", err)
	}
	defer file.Close()

	req := openai.AudioRequest{
		Model:    openai.Whisper1,
		Reader:   file,
		FilePath: filename,
		Language: "en",
	}

	resp, err := ap.openAIClient.CreateTranscription(context.Background(), req)
	if err != nil {
		return "", fmt.Errorf("failed to transcribe audio: %v", err)
	}

	return resp.Text, nil
}

func (ap *AudioProcessor) synthesizeSpeech(text string, config *Config) error {
	// Set isPlaying to true before playing audio
	ap.isPlaying = true
	defer func() {
		ap.isPlaying = false
		// Add a tiny delay to let the audio system stabilize
		time.Sleep(time.Millisecond * 100)
	}()

	// Get available voices if voice ID is not specified
	var voiceID string
	if config.ElevenLabsVoiceID != "" {
		voiceID = config.ElevenLabsVoiceID
	} else {
		voices, err := ap.elevenLabsClient.GetVoices()
		if err != nil {
			return fmt.Errorf("failed to get voices: %v", err)
		}

		// Find Nicole voice or use the first available voice
		for _, voice := range voices {
			if voice.Name == "Nicole" {
				voiceID = voice.VoiceId
				break
			}
		}
		if voiceID == "" && len(voices) > 0 {
			voiceID = voices[0].VoiceId
		}
	}

	// Create temporary file for the response
	tempFile := "response.mp3"
	out, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer func() {
		out.Close()
		os.Remove(tempFile)
	}()

	// Create text-to-speech request
	ttsReq := elevenlabs.TextToSpeechRequest{
		Text:    text,
		ModelID: config.ElevenLabsModelID,
	}

	// Generate speech
	err = ap.elevenLabsClient.TextToSpeechStream(out, voiceID, ttsReq)
	if err != nil {
		return fmt.Errorf("failed to generate speech: %v", err)
	}

	// For simplicity, we'll use the system's default audio player
	return ap.playAudioFile(tempFile)
}

func (ap *AudioProcessor) playAudioFile(filename string) error {
	var cmd string
	switch os := runtime.GOOS; os {
	case "darwin":
		cmd = "afplay"
	case "linux":
		cmd = "aplay"
	case "windows":
		cmd = "start"
	default:
		return fmt.Errorf("unsupported operating system")
	}

	command := exec.Command(cmd, filename)
	return command.Run()
}

func (ap *AudioProcessor) cleanup() {
	if ap.stream != nil {
		ap.stream.Stop()
		ap.stream.Close()
		ap.stream = nil
	}
	portaudio.Terminate()
}

func main() {
	fmt.Println("Loading configuration...")
	config, err := LoadConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		return
	}

	fmt.Println("Initializing audio processor...")
	processor, err := NewAudioProcessor(config)
	if err != nil {
		fmt.Printf("Failed to initialize audio processor: %v\n", err)
		return
	}
	defer processor.cleanup()

	fmt.Println("\nAudio settings:")
	fmt.Printf("Sample Rate: %d Hz\n", config.SampleRate)
	fmt.Printf("Frames Per Buffer: %d\n", config.FramesPerBuffer)
	fmt.Printf("Channels: %d\n", config.Channels)

	fmt.Println("\nConversation started! You can start speaking when you see 'Listening...'")

	for {
		// Small pause to separate conversation turns
		time.Sleep(time.Millisecond * 100)
		fmt.Print("\n\nListening... ")

		err = processor.startRecording()
		if err != nil {
			fmt.Printf("Failed to start recording: %v\n", err)
			continue
		}

		err = processor.processAudio()
		if err != nil {
			fmt.Printf("Failed to process audio: %v\n", err)
			continue
		}

		// Stop and close the stream after recording
		if processor.stream != nil {
			processor.stream.Stop()
			processor.stream.Close()
			processor.stream = nil
		}

		fmt.Println("Saving audio file...")
		err = processor.saveWAV("voice_record.wav")
		if err != nil {
			fmt.Printf("Failed to save WAV file: %v\n", err)
			continue
		}

		fmt.Println("Transcribing audio...")
		userText, err := processor.transcribeAudio("voice_record.wav")
		if err != nil {
			fmt.Printf("Failed to transcribe audio: %v\n", err)
			continue
		}
		fmt.Printf("You said: %s\n", userText)

		fmt.Println("Sophia is thinking...")
		response, err, shouldExit := processor.generateResponse(userText)
		if err != nil {
			fmt.Printf("Failed to generate response: %v\n", err)
			continue
		}

		fmt.Println("\nSophia is speaking...")
		err = processor.synthesizeSpeech(response, config)
		if err != nil {
			fmt.Printf("Failed to synthesize speech: %v\n", err)
			continue
		}

		// Check if we should exit after Sophia's response
		if shouldExit {
			fmt.Println("\nGoodbye! Conversation ended.")
			return
		}
	}
}
