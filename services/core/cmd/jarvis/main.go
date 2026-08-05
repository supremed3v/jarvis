package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
	types "jarvis-pa/packages/shared-types"

	core "jarvis-pa/services/core"
	"jarvis-pa/services/core/voice"
)

const defaultAgentID = "jarvis"

func main() {
	configPath := ""
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	log := logger.New("jarvis", logger.WithMinLevel(logger.LevelInfo))

	cfg, err := cfgpkg.Load(configPath)
	if err != nil {
		log.Error("failed to load config", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	bus := core.NewBus()
	registry := core.NewRegistry()
	lifecycle := core.NewLifecycleManager(registry)

	ollama := core.NewOllamaProvider()
	ollamaURL := fmt.Sprintf("http://%s:%d", cfg.Model.OllamaHost, cfg.Model.OllamaPort)
	if err := ollama.Configure(core.ProviderConfig{
		BaseURL: ollamaURL,
		Timeout: 120 * time.Second,
	}); err != nil {
		log.Error("failed to configure ollama", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	modelRouter := core.NewModelRouter(cfg.Model, ollama, log, nil)
	streamHandler := core.NewStreamHandler(ollama, log)
	promptRegistry := core.NewPromptRegistry()
	windowManager := core.NewWindowManager()
	budgetManager := core.NewBudgetManager(cfg.Model)

	localStore := core.NewLocalStore()
	memory := core.NewStorageMemory(localStore)
	conversationMemory := core.NewConversationMemory(memory)
	userProfileMemory := core.NewUserProfileMemory(memory)
	memoryRetriever := core.NewMemoryRetriever(memory)
	consolidationEngine := core.NewConsolidationEngine(memory)
	approvalQueue := core.NewApprovalQueue()

	memoryViewer := core.NewCoreMemoryViewer(memory,
		core.WithViewerConversations(conversationMemory),
		core.WithViewerProfile(userProfileMemory),
	)

	defaultModel, err := cfg.Model.ModelFor(defaultAgentID)
	if err != nil {
		defaultModel, err = cfg.Model.ModelFor("")
		if err != nil || defaultModel.Name == "" {
			defaultModel = cfgpkg.Model{
				Name:        "qwen2.5-coder:14b",
				Temperature: 0.7,
			}
		}
	}
	if defaultModel.Options == nil {
		defaultModel.Options = map[string]any{}
	}
	if _, ok := defaultModel.Options["num_ctx"]; !ok {
		defaultModel.Options["num_ctx"] = 4096
	}
	if _, ok := defaultModel.Options["temperature"]; !ok {
		defaultModel.Options["temperature"] = 0.3
	}
	if _, ok := defaultModel.Options["num_predict"]; !ok {
		defaultModel.Options["num_predict"] = 256
	}

	// --- Voice pipeline ---

	// Resolve relative model paths against the executable's directory (or
	// the repo root when run via `go run`), so the runtime works regardless
	// of which directory it is started from.
	repoRoot := resolveRepoRoot()
	if !filepath.IsAbs(cfg.Voice.WakeWordModelPath) {
		cfg.Voice.WakeWordModelPath = filepath.Join(repoRoot, cfg.Voice.WakeWordModelPath)
	}
	if !filepath.IsAbs(cfg.Voice.TTSModel) {
		cfg.Voice.TTSModel = filepath.Join(repoRoot, cfg.Voice.TTSModel)
	}

	audioEngine, err := voice.NewAudioEngine(&cfg.Voice, log)
	if err != nil {
		log.Error("failed to create audio engine", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	if err := audioEngine.Initialize(&cfg.Voice, log); err != nil {
		log.Error("failed to initialize audio engine", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	wakeWordDetector, err := voice.NewWakeWordDetector(cfg.Voice.WakeWordModelPath, cfg.Voice.PythonPath)
	if err != nil {
		log.Error("failed to create wake word detector", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	whisperProvider, err := voice.NewWhisperProvider(&cfg.Voice, log)
	if err != nil {
		log.Error("failed to create whisper provider", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	piperProvider := voice.NewPiperProvider(&cfg.Voice, log)

	mic := voice.NewMicrophone(audioEngine, wakeWordDetector, whisperProvider, bus, log)

	wakeWordPattern := regexp.MustCompile(`(?i)^hey[,.]?\s*jarvis[,!.?]*\s*`)
	stripWakeWord := func(transcript string) string {
		cleaned := wakeWordPattern.ReplaceAllString(transcript, "")
		return strings.TrimSpace(cleaned)
	}

	mdBold := regexp.MustCompile(`\*+([^*]+)\*+`)
	mdHeader := regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdBullet := regexp.MustCompile(`(?m)^[\s]*[-*]\s+`)
	mdNumbered := regexp.MustCompile(`(?m)^[\s]*\d+\.\s+`)
	mdCode := regexp.MustCompile("`[^`]*`")
	stripMarkdown := func(text string) string {
		text = mdBold.ReplaceAllString(text, "$1")
		text = mdHeader.ReplaceAllString(text, "")
		text = mdBullet.ReplaceAllString(text, "")
		text = mdNumbered.ReplaceAllString(text, "")
		text = mdCode.ReplaceAllString(text, "")
		return strings.TrimSpace(text)
	}

	const voiceSystemPrompt = "You are JARVIS, a helpful personal AI assistant. " +
		"Respond conversationally in plain text. Be brief and direct. " +
		"Never use markdown formatting, asterisks, bullet points, numbered lists, or code blocks. " +
		"Keep answers to 2-3 sentences unless the user asks for detail."

	voiceRequestHandler := func(ctx context.Context, transcript string) (string, error) {
		prompt := stripWakeWord(transcript)
		if prompt == "" {
			return "Hello sir, how can I help you?", nil
		}
		log.Info("voice request", map[string]any{"transcript": prompt})
		resp, genErr := ollama.Generate(ctx, core.GenerateRequest{
			Model:   defaultModel.Name,
			Prompt:  prompt,
			System:  voiceSystemPrompt,
			Options: defaultModel.Options,
		})
		if genErr != nil {
			return "", genErr
		}
		return stripMarkdown(resp.Text), nil
	}

	voiceStreamingHandler := func(ctx context.Context, transcript string, onChunk func(voice.ResponseChunk) error) error {
		prompt := stripWakeWord(transcript)
		if prompt == "" {
			return onChunk(voice.ResponseChunk{Text: "Hello sir, how can I help you?", Done: true})
		}
		log.Info("voice streaming request", map[string]any{"transcript": prompt})
		return ollama.Stream(ctx, core.GenerateRequest{
			Model:   defaultModel.Name,
			Prompt:  prompt,
			System:  voiceSystemPrompt,
			Options: defaultModel.Options,
		}, func(chunk core.StreamChunk) error {
			return onChunk(voice.ResponseChunk{
				Text: stripMarkdown(chunk.Text),
				Done: chunk.Done,
			})
		})
	}

	sessionManager, err := voice.NewSessionManager(
		mic, audioEngine, piperProvider, &cfg.Voice, bus,
		voiceRequestHandler, log,
		voice.WithStreamingHandler(voiceStreamingHandler),
	)
	if err != nil {
		log.Error("failed to create voice session manager", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	// After any session completes, auto re-enter listening mode so the user
	// can keep talking without repeating the wake word each time. The
	// 15-second session timeout handles the "user walked away" case.
	bus.Subscribe(voice.EventSessionCompleted, func(event types.Event) {
		log.Info("voice: re-entering listening mode", nil)
		time.Sleep(500 * time.Millisecond)
		bus.Publish(types.Event{
			Type:      voice.EventWakeWordDetected,
			Source:    "jarvis.continue-listening",
			Timestamp: time.Now().UTC(),
		})
	})

	// --- Desktop command handler ---

	desktopStreamingHandler := func(ctx context.Context, cmd core.Command, onChunk func(core.CommandChunk) error) error {
		log.Info("command received", map[string]any{"id": cmd.ID, "text": cmd.Text})
		return ollama.Stream(ctx, core.GenerateRequest{
			Model:   defaultModel.Name,
			Prompt:  cmd.Text,
			Options: defaultModel.Options,
		}, func(chunk core.StreamChunk) error {
			return onChunk(core.CommandChunk{
				Text: chunk.Text,
				Done: chunk.Done,
			})
		})
	}

	// --- Agent ---

	manifest := &core.Manifest{
		Name:        defaultAgentID,
		Description: "Default personal assistant agent",
		Tools:       []string{},
	}
	jarvisAgent, err := core.NewAgentFromManifest(manifest, func(ctx context.Context, task *types.Task) (map[string]any, error) {
		resp, genErr := ollama.Generate(ctx, core.GenerateRequest{
			Model:   defaultModel.Name,
			Prompt:  task.Title,
			Options: defaultModel.Options,
		})
		if genErr != nil {
			return nil, genErr
		}
		return map[string]any{"response": resp.Text}, nil
	})
	if err != nil {
		log.Error("failed to create jarvis agent", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	if err := registry.Register(jarvisAgent); err != nil {
		log.Error("failed to register jarvis agent", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	// --- Bridge ---

	bridge := core.NewBridge(
		core.WithBridgeEventBus(bus),
		core.WithBridgeLogger(log),
		core.WithBridgeVersion("0.1.0"),
		core.WithStreamingCommandHandler(desktopStreamingHandler),
		core.WithBridgeAgentRegistry(registry, defaultAgentID),
		core.WithApprovalQueue(approvalQueue),
		core.WithBridgeLifecycleManager(lifecycle),
		core.WithBridgeMemory(memoryViewer),
		core.WithBridgeVoiceSessionManager(sessionManager),
	)

	// --- Container (holds references for future use) ---

	container := core.NewContainer(cfg, log,
		core.WithEventBus(bus),
		core.WithAgentRegistry(registry),
		core.WithProvider(ollama),
		core.WithRouter(modelRouter),
		core.WithStreamHandler(streamHandler),
		core.WithPromptRegistry(promptRegistry),
		core.WithWindowManager(windowManager),
		core.WithBudgetManager(budgetManager),
		core.WithMemory(memory),
		core.WithMemoryRetriever(memoryRetriever),
		core.WithConsolidationEngine(consolidationEngine),
		core.WithWSBridge(bridge),
		core.WithVoiceEngine(audioEngine),
		core.WithSTTProvider(whisperProvider),
		core.WithTTSProvider(piperProvider),
		core.WithVoiceSessionManager(sessionManager),
	)
	_ = container

	// --- Runtime ---

	rt := core.New(
		core.WithConfigPath(configPath),
		core.WithDependencies(bridge),
	)

	log.Info("=== JARVIS Runtime ===", nil)
	log.Info("subsystems active", map[string]any{
		"ollama":   ollamaURL,
		"bridge":   "127.0.0.1:42321",
		"agent":    defaultAgentID,
		"model":    defaultModel.Name,
		"num_ctx":  defaultModel.Options["num_ctx"],
		"memory":   "local-store",
		"voice":    "enabled",
		"stt":      fmt.Sprintf("whisper/%s", cfg.Voice.STTModel),
		"tts":      fmt.Sprintf("piper/%s", cfg.Voice.TTSModel),
		"wakeWord": cfg.Voice.WakeWordModelPath,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Info("starting runtime (Ctrl+C to stop)", nil)
	if err := rt.Run(ctx); err != nil {
		log.Error("runtime exited with error", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	audioEngine.Shutdown()
	log.Info("runtime stopped cleanly", nil)
}

// resolveRepoRoot walks up from the current working directory looking for
// go.work (the repo root marker). Falls back to cwd if not found.
func resolveRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	cwd, _ := os.Getwd()
	return cwd
}
