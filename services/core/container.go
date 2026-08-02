// container.go implements SPEC-0008: the dependency container. Container
// gives runtime components typed access to shared services through an
// explicit, instance-scoped struct rather than package-level globals.
package core

import (
	"context"

	cfgpkg "jarvis-pa/packages/config"
	"jarvis-pa/packages/logger"
)

// VoiceEngine provides audio capture and playback (SPEC-0053).
type VoiceEngine interface {
	Initialize(cfg *cfgpkg.VoiceConfig, log *logger.Logger) error
	Capture() (<-chan []byte, error)
	Playback(audio []byte) error
	Shutdown() error
}

// WakeWordDetector detects wake word activations (SPEC-0055). Start takes
// audioCh (the same shape STTProvider.StreamTranscribe's audioCh parameter
// uses) as the source of audio to scan for the wake word - an implementation
// consumes it internally rather than exposing a separate write method, so a
// caller only ever needs Start/Stop to drive it.
type WakeWordDetector interface {
	Start(ctx context.Context, audioCh <-chan []byte, onDetect func()) error
	Stop() error
}

// STTProvider converts speech to text (SPEC-0057).
type STTProvider interface {
	Transcribe(ctx context.Context, audio []byte) (string, error)
	StreamTranscribe(ctx context.Context, audioCh <-chan []byte, textCh chan<- string) error
}

// TTSProvider converts text to speech (SPEC-0059).
type TTSProvider interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
	StreamSynthesize(ctx context.Context, text string, audioCh chan<- []byte) error
}

// TerminalTool executes terminal commands and manages PTY sessions (SPEC-0050).
type TerminalTool interface {
	Execute(ctx context.Context, cmd string, args []string) (string, error)
	StartSession(ctx context.Context, sessionID string) (TerminalSession, error)
	StartClaudeCode(ctx context.Context, sessionID, projectPath string) (TerminalSession, error)
}

// TerminalSession is a persistent PTY session (SPEC-0050).
type TerminalSession interface {
	Write(data []byte) error
	Read() ([]byte, error)
	Close() error
	Resize(cols, rows int) error
}

// FilesystemTool provides sandboxed filesystem access (SPEC-0049).
type FilesystemTool interface {
	ReadFile(ctx context.Context, path string) (string, error)
	WriteFile(ctx context.Context, path, content string) error
	ListDir(ctx context.Context, path string) ([]FileInfo, error)
	Glob(ctx context.Context, pattern string) ([]string, error)
}

// FileInfo describes a filesystem entry (SPEC-0049).
type FileInfo struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime int64
}

// WSBridge handles WebSocket communication with the desktop app (SPEC-0065).
type WSBridge interface {
	Start(addr string) error
	Stop() error
	Broadcast(event string, payload any) error
}

// TaskManager is a placeholder slot for the Task Execution layer
// (SPEC-0011..0017). Not yet implemented.
type TaskManager interface{}

// ToolRegistry is a placeholder slot for the SPEC-0045 Tool Registry.
// Not yet implemented.
type ToolRegistry interface{}

// Container holds the shared services a Core Runtime component may depend
// on. Config, Logger, EventBus, AgentRegistry, Provider, Router,
// StreamHandler, PromptRegistry, WindowManager, BudgetManager, Memory,
// EmbeddingPipeline, KnowledgeIngestionPipeline, and MemoryRetriever are
// wired to their real SPEC-0003, SPEC-0005, SPEC-0009, SPEC-0020,
// SPEC-0026/0027, SPEC-0029, SPEC-0030, SPEC-0031, SPEC-0032, SPEC-0033,
// SPEC-0034/0035, SPEC-0039, SPEC-0040, and SPEC-0041 implementations; the
// remaining slots are typed placeholders until their owning specs are
// implemented. Every slot stays nil unless supplied via options.
type Container struct {
	Config *cfgpkg.Config
	Logger *logger.Logger

	EventBus                   EventBus
	TaskManager                TaskManager
	ToolRegistry               ToolRegistry
	AgentRegistry              AgentRegistry
	Provider                   Provider
	Router                     *ModelRouter
	StreamHandler              *StreamHandler
	PromptRegistry             *PromptRegistry
	WindowManager              *WindowManager
	BudgetManager              *BudgetManager
	Memory                     Memory
	EmbeddingPipeline          *EmbeddingPipeline
	KnowledgeIngestionPipeline *KnowledgeIngestionPipeline
	MemoryRetriever            *MemoryRetriever

	VoiceEngine      VoiceEngine
	WakeWordDetector WakeWordDetector
	STTProvider      STTProvider
	TTSProvider      TTSProvider
	TerminalTool     TerminalTool
	FilesystemTool   FilesystemTool
	WSBridge         WSBridge
}

// ContainerOption configures a Container created by NewContainer.
type ContainerOption func(*Container)

// WithEventBus sets the Container's Event Bus slot.
func WithEventBus(b EventBus) ContainerOption {
	return func(c *Container) { c.EventBus = b }
}

// WithTaskManager sets the Container's Task Manager slot.
func WithTaskManager(m TaskManager) ContainerOption {
	return func(c *Container) { c.TaskManager = m }
}

// WithToolRegistry sets the Container's Tool Registry slot.
func WithToolRegistry(r ToolRegistry) ContainerOption {
	return func(c *Container) { c.ToolRegistry = r }
}

// WithAgentRegistry sets the Container's Agent Registry slot.
func WithAgentRegistry(r AgentRegistry) ContainerOption {
	return func(c *Container) { c.AgentRegistry = r }
}

// WithProvider sets the Container's LLM Provider slot.
func WithProvider(p Provider) ContainerOption {
	return func(c *Container) { c.Provider = p }
}

// WithRouter sets the Container's Model Router slot.
func WithRouter(r *ModelRouter) ContainerOption {
	return func(c *Container) { c.Router = r }
}

// WithStreamHandler sets the Container's Streaming Response Handler slot.
func WithStreamHandler(h *StreamHandler) ContainerOption {
	return func(c *Container) { c.StreamHandler = h }
}

// WithPromptRegistry sets the Container's Prompt Registry slot.
func WithPromptRegistry(r *PromptRegistry) ContainerOption {
	return func(c *Container) { c.PromptRegistry = r }
}

// WithWindowManager sets the Container's Context Window Manager slot.
func WithWindowManager(w *WindowManager) ContainerOption {
	return func(c *Container) { c.WindowManager = w }
}

// WithBudgetManager sets the Container's Token Budget Manager slot.
func WithBudgetManager(m *BudgetManager) ContainerOption {
	return func(c *Container) { c.BudgetManager = m }
}

// WithMemory sets the Container's Memory slot.
func WithMemory(m Memory) ContainerOption {
	return func(c *Container) { c.Memory = m }
}

// WithEmbeddingPipeline sets the Container's Embedding Pipeline slot.
func WithEmbeddingPipeline(p *EmbeddingPipeline) ContainerOption {
	return func(c *Container) { c.EmbeddingPipeline = p }
}

// WithKnowledgeIngestionPipeline sets the Container's Knowledge Ingestion
// Pipeline slot.
func WithKnowledgeIngestionPipeline(p *KnowledgeIngestionPipeline) ContainerOption {
	return func(c *Container) { c.KnowledgeIngestionPipeline = p }
}

// WithMemoryRetriever sets the Container's Memory Retriever slot.
func WithMemoryRetriever(r *MemoryRetriever) ContainerOption {
	return func(c *Container) { c.MemoryRetriever = r }
}

// WithVoiceEngine sets the Container's Voice Engine slot.
func WithVoiceEngine(v VoiceEngine) ContainerOption {
	return func(c *Container) { c.VoiceEngine = v }
}

// WithWakeWordDetector sets the Container's Wake Word Detector slot.
func WithWakeWordDetector(w WakeWordDetector) ContainerOption {
	return func(c *Container) { c.WakeWordDetector = w }
}

// WithSTTProvider sets the Container's STT Provider slot.
func WithSTTProvider(s STTProvider) ContainerOption {
	return func(c *Container) { c.STTProvider = s }
}

// WithTTSProvider sets the Container's TTS Provider slot.
func WithTTSProvider(t TTSProvider) ContainerOption {
	return func(c *Container) { c.TTSProvider = t }
}

// WithTerminalTool sets the Container's Terminal Tool slot.
func WithTerminalTool(t TerminalTool) ContainerOption {
	return func(c *Container) { c.TerminalTool = t }
}

// WithFilesystemTool sets the Container's Filesystem Tool slot.
func WithFilesystemTool(f FilesystemTool) ContainerOption {
	return func(c *Container) { c.FilesystemTool = f }
}

// WithWSBridge sets the Container's WebSocket Bridge slot.
func WithWSBridge(w WSBridge) ContainerOption {
	return func(c *Container) { c.WSBridge = w }
}

// NewContainer creates a Container wired to the given Config and Logger.
// cfg and log are required; the remaining slots default to nil and are set
// only via the supplied options.
func NewContainer(cfg *cfgpkg.Config, log *logger.Logger, opts ...ContainerOption) *Container {
	c := &Container{
		Config: cfg,
		Logger: log,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}
