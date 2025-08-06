package app

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/pkg/logger"
)

// Context represents the application execution context
type Context struct {
	// Core context
	ctx    context.Context
	cancel context.CancelFunc

	// Configuration
	config *config.Config
	logger logger.Logger

	// Application state
	state   *ApplicationState
	stateMu sync.RWMutex

	// Event system
	events *EventManager

	// Resource tracking
	resources *ResourceTracker

	// Session information
	session *SessionInfo

	// Lifecycle hooks
	hooks *LifecycleHooks
}

// ApplicationState represents the current state of the application
type ApplicationState struct {
	// Initialization state
	Initialized   bool      `json:"initialized"`
	InitializedAt time.Time `json:"initialized_at"`

	// Discovery state
	DiscoveryComplete   bool      `json:"discovery_complete"`
	DiscoveryStartedAt  time.Time `json:"discovery_started_at"`
	DiscoveryFinishedAt time.Time `json:"discovery_finished_at"`

	// Task execution state
	ActiveTasks     int       `json:"active_tasks"`
	TotalExecuted   int64     `json:"total_executed"`
	LastTaskStarted time.Time `json:"last_task_started"`

	// Application mode
	Mode ApplicationMode `json:"mode"`

	// Error state
	LastError     error     `json:"last_error,omitempty"`
	LastErrorTime time.Time `json:"last_error_time,omitempty"`

	// Performance metrics
	MemoryUsage int64   `json:"memory_usage"`
	CPUUsage    float64 `json:"cpu_usage"`
}

// ApplicationMode represents different modes of operation
type ApplicationMode int

const (
	ModeCLI ApplicationMode = iota
	ModeTUI
	ModeAPI
	ModeDaemon
)

// String returns the string representation of the application mode
func (m ApplicationMode) String() string {
	switch m {
	case ModeCLI:
		return "cli"
	case ModeTUI:
		return "tui"
	case ModeAPI:
		return "api"
	case ModeDaemon:
		return "daemon"
	default:
		return "unknown"
	}
}

// SessionInfo contains information about the current session
type SessionInfo struct {
	// Session identification
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`

	// Process information
	PID            int    `json:"pid"`
	WorkingDir     string `json:"working_dir"`
	ExecutablePath string `json:"executable_path"`

	// User information
	UserName string `json:"user_name"`
	UserID   string `json:"user_id"`

	// Environment
	Environment map[string]string `json:"environment"`

	// Command line arguments
	Args []string `json:"args"`

	// Terminal information
	IsTerminal   bool   `json:"is_terminal"`
	TerminalType string `json:"terminal_type,omitempty"`
	TerminalSize struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"terminal_size,omitempty"`
}

// EventManager handles application-wide events
type EventManager struct {
	logger logger.Logger

	// Event channels
	appEvents   chan *ApplicationEvent
	taskEvents  chan *TaskEvent
	stateEvents chan *StateEvent

	// Subscribers
	appSubscribers   []chan *ApplicationEvent
	taskSubscribers  []chan *TaskEvent
	stateSubscribers []chan *StateEvent

	// Control
	mu       sync.RWMutex
	shutdown chan struct{}
	done     chan struct{}
}

// ApplicationEvent represents application-level events
type ApplicationEvent struct {
	Type      ApplicationEventType `json:"type"`
	Timestamp time.Time            `json:"timestamp"`
	Data      any                  `json:"data,omitempty"`
	Error     error                `json:"error,omitempty"`
}

// ApplicationEventType represents types of application events
type ApplicationEventType int

const (
	EventAppStarted ApplicationEventType = iota
	EventAppShutdown
	EventAppError
	EventConfigChanged
	EventDiscoveryStarted
	EventDiscoveryCompleted
	EventModeChanged
)

// String returns the string representation of the event type
func (e ApplicationEventType) String() string {
	switch e {
	case EventAppStarted:
		return "app_started"
	case EventAppShutdown:
		return "app_shutdown"
	case EventAppError:
		return "app_error"
	case EventConfigChanged:
		return "config_changed"
	case EventDiscoveryStarted:
		return "discovery_started"
	case EventDiscoveryCompleted:
		return "discovery_completed"
	case EventModeChanged:
		return "mode_changed"
	default:
		return "unknown"
	}
}

// StateEvent represents application state changes
type StateEvent struct {
	Type      StateEventType `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	OldValue  any            `json:"old_value,omitempty"`
	NewValue  any            `json:"new_value,omitempty"`
}

// StateEventType represents types of state events
type StateEventType int

const (
	StateEventInitialized StateEventType = iota
	StateEventTaskCountChanged
	StateEventMemoryChanged
	StateEventErrorOccurred
)

// ResourceTracker tracks application resource usage
type ResourceTracker struct {
	logger logger.Logger

	// Resource metrics
	startTime     time.Time
	memoryPeak    int64
	memoryHistory []int64

	// Goroutine tracking
	goroutineCount   int
	goroutineHistory []int

	// File descriptor tracking
	openFiles int
	maxFiles  int

	// Control
	interval time.Duration
	stopChan chan struct{}
	done     chan struct{}
	mu       sync.RWMutex
}

// LifecycleHooks defines hooks for application lifecycle events
type LifecycleHooks struct {
	// Initialization hooks
	PreInit  []func(*Context) error
	PostInit []func(*Context) error

	// Shutdown hooks
	PreShutdown  []func(*Context) error
	PostShutdown []func(*Context) error

	// Error hooks
	OnError []func(*Context, error)

	// State change hooks
	OnStateChange []func(*Context, *ApplicationState)
}

// NewContext creates a new application context
func NewContext(cfg *config.Config, log logger.Logger) (*Context, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Create base context
	ctx, cancel := context.WithCancel(context.Background())

	// Create session info
	session, err := createSessionInfo()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create session info: %w", err)
	}

	// Create event manager
	eventManager := NewEventManager(log)

	// Create resource tracker
	resourceTracker := NewResourceTracker(log)

	appCtx := &Context{
		ctx:    ctx,
		cancel: cancel,
		config: cfg,
		logger: log.WithGroup("app-context"),
		state: &ApplicationState{
			Mode: ModeCLI, // Default mode
		},
		events:    eventManager,
		resources: resourceTracker,
		session:   session,
		hooks:     &LifecycleHooks{},
	}

	// Start background services
	if err := appCtx.startBackgroundServices(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start background services: %w", err)
	}

	return appCtx, nil
}

// createSessionInfo creates session information
func createSessionInfo() (*SessionInfo, error) {
	// Generate session ID
	sessionID := fmt.Sprintf("wake-%d", time.Now().UnixNano())

	// Get process information
	pid := os.Getpid()

	// Get working directory
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = "unknown"
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		execPath = "unknown"
	}

	// Get user information
	userName := os.Getenv("USER")
	if userName == "" {
		userName = os.Getenv("USERNAME") // Windows
	}
	if userName == "" {
		userName = "unknown"
	}

	userID := os.Getenv("UID")
	if userID == "" {
		userID = "unknown"
	}

	// Get environment variables
	env := make(map[string]string)
	for _, envVar := range os.Environ() {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	// Get command line arguments
	args := os.Args

	// Check if we're running in a terminal
	isTerminal := isRunningInTerminal()

	session := &SessionInfo{
		ID:             sessionID,
		StartTime:      time.Now(),
		PID:            pid,
		WorkingDir:     workingDir,
		ExecutablePath: execPath,
		UserName:       userName,
		UserID:         userID,
		Environment:    env,
		Args:           args,
		IsTerminal:     isTerminal,
	}

	// Get terminal information if available
	if isTerminal {
		session.TerminalType = os.Getenv("TERM")
		// Terminal size would require platform-specific code
	}

	return session, nil
}

// isRunningInTerminal checks if the application is running in a terminal
func isRunningInTerminal() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// Initialize initializes the application context
func (c *Context) Initialize() error {
	c.logger.Debug("Initializing application context")

	// Run pre-initialization hooks
	for _, hook := range c.hooks.PreInit {
		if err := hook(c); err != nil {
			return fmt.Errorf("pre-init hook failed: %w", err)
		}
	}

	// Update state
	c.stateMu.Lock()
	c.state.Initialized = true
	c.state.InitializedAt = time.Now()
	c.stateMu.Unlock()

	// Emit initialization event
	c.events.EmitApplicationEvent(EventAppStarted, nil, nil)

	// Run post-initialization hooks
	for _, hook := range c.hooks.PostInit {
		if err := hook(c); err != nil {
			c.logger.Warn("Post-init hook failed", "error", err)
		}
	}

	c.logger.Info("Application context initialized")
	return nil
}

// Shutdown gracefully shuts down the application context
func (c *Context) Shutdown() error {
	c.logger.Info("Shutting down application context")

	// Run pre-shutdown hooks
	for _, hook := range c.hooks.PreShutdown {
		if err := hook(c); err != nil {
			c.logger.Warn("Pre-shutdown hook failed", "error", err)
		}
	}

	// Emit shutdown event
	c.events.EmitApplicationEvent(EventAppShutdown, nil, nil)

	// Stop background services
	c.stopBackgroundServices()

	// Cancel context
	c.cancel()

	// Run post-shutdown hooks
	for _, hook := range c.hooks.PostShutdown {
		if err := hook(c); err != nil {
			c.logger.Warn("Post-shutdown hook failed", "error", err)
		}
	}

	return nil
}

// Context returns the underlying context.Context
func (c *Context) Context() context.Context {
	return c.ctx
}

// Config returns the application configuration
func (c *Context) Config() *config.Config {
	return c.config
}

// Logger returns the application logger
func (c *Context) Logger() logger.Logger {
	return c.logger
}

// GetState returns a copy of the current application state
func (c *Context) GetState() *ApplicationState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()

	// Return a copy to prevent external modification
	state := *c.state
	return &state
}

// SetMode sets the application mode
func (c *Context) SetMode(mode ApplicationMode) {
	c.stateMu.Lock()
	oldMode := c.state.Mode
	c.state.Mode = mode
	c.stateMu.Unlock()

	if oldMode != mode {
		c.events.EmitApplicationEvent(EventModeChanged, map[string]any{
			"old_mode": oldMode.String(),
			"new_mode": mode.String(),
		}, nil)
	}
}

// UpdateTaskCount updates the active task count
func (c *Context) UpdateTaskCount(delta int) {
	c.stateMu.Lock()
	oldCount := c.state.ActiveTasks
	c.state.ActiveTasks += delta

	if delta > 0 {
		c.state.TotalExecuted++
		c.state.LastTaskStarted = time.Now()
	}
	c.stateMu.Unlock()

	// Emit state change event
	c.events.EmitStateEvent(StateEventTaskCountChanged, oldCount, c.state.ActiveTasks)
}

// ReportError reports an error to the context
func (c *Context) ReportError(err error) {
	if err == nil {
		return
	}

	c.stateMu.Lock()
	c.state.LastError = err
	c.state.LastErrorTime = time.Now()
	c.stateMu.Unlock()

	// Emit error event
	c.events.EmitApplicationEvent(EventAppError, nil, err)

	// Run error hooks
	for _, hook := range c.hooks.OnError {
		hook(c, err)
	}

	c.logger.Error("Application error reported", "error", err)
}

// GetSession returns session information
func (c *Context) GetSession() *SessionInfo {
	return c.session
}

// SubscribeToApplicationEvents subscribes to application events
func (c *Context) SubscribeToApplicationEvents() <-chan *ApplicationEvent {
	return c.events.SubscribeToApplicationEvents()
}

// SubscribeToTaskEvents subscribes to task events
func (c *Context) SubscribeToTaskEvents() <-chan *TaskEvent {
	return c.events.SubscribeToTaskEvents()
}

// SubscribeToStateEvents subscribes to state events
func (c *Context) SubscribeToStateEvents() <-chan *StateEvent {
	return c.events.SubscribeToStateEvents()
}

// AddPreInitHook adds a pre-initialization hook
func (c *Context) AddPreInitHook(hook func(*Context) error) {
	c.hooks.PreInit = append(c.hooks.PreInit, hook)
}

// AddPostInitHook adds a post-initialization hook
func (c *Context) AddPostInitHook(hook func(*Context) error) {
	c.hooks.PostInit = append(c.hooks.PostInit, hook)
}

// AddPreShutdownHook adds a pre-shutdown hook
func (c *Context) AddPreShutdownHook(hook func(*Context) error) {
	c.hooks.PreShutdown = append(c.hooks.PreShutdown, hook)
}

// AddPostShutdownHook adds a post-shutdown hook
func (c *Context) AddPostShutdownHook(hook func(*Context) error) {
	c.hooks.PostShutdown = append(c.hooks.PostShutdown, hook)
}

// AddErrorHook adds an error handler hook
func (c *Context) AddErrorHook(hook func(*Context, error)) {
	c.hooks.OnError = append(c.hooks.OnError, hook)
}

// startBackgroundServices starts background services
func (c *Context) startBackgroundServices() error {
	// Start event manager
	if err := c.events.Start(); err != nil {
		return fmt.Errorf("failed to start event manager: %w", err)
	}

	// Start resource tracker
	if err := c.resources.Start(); err != nil {
		return fmt.Errorf("failed to start resource tracker: %w", err)
	}

	return nil
}

// stopBackgroundServices stops background services
func (c *Context) stopBackgroundServices() {
	c.events.Stop()
	c.resources.Stop()
}

// NewEventManager creates a new event manager
func NewEventManager(log logger.Logger) *EventManager {
	return &EventManager{
		logger:      log.WithGroup("events"),
		appEvents:   make(chan *ApplicationEvent, 100),
		taskEvents:  make(chan *TaskEvent, 100),
		stateEvents: make(chan *StateEvent, 100),
		shutdown:    make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// Start starts the event manager
func (em *EventManager) Start() error {
	go em.eventLoop()
	return nil
}

// Stop stops the event manager
func (em *EventManager) Stop() {
	close(em.shutdown)
	<-em.done
}

// eventLoop processes events
func (em *EventManager) eventLoop() {
	defer close(em.done)

	for {
		select {
		case <-em.shutdown:
			return

		case appEvent := <-em.appEvents:
			em.distributeApplicationEvent(appEvent)

		case taskEvent := <-em.taskEvents:
			em.distributeTaskEvent(taskEvent)

		case stateEvent := <-em.stateEvents:
			em.distributeStateEvent(stateEvent)
		}
	}
}

// EmitApplicationEvent emits an application event
func (em *EventManager) EmitApplicationEvent(eventType ApplicationEventType, data any, err error) {
	event := &ApplicationEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		Data:      data,
		Error:     err,
	}

	select {
	case em.appEvents <- event:
	default:
		em.logger.Warn("Application event channel full, dropping event", "type", eventType.String())
	}
}

// EmitStateEvent emits a state change event
func (em *EventManager) EmitStateEvent(eventType StateEventType, oldValue, newValue any) {
	event := &StateEvent{
		Type:      eventType,
		Timestamp: time.Now(),
		OldValue:  oldValue,
		NewValue:  newValue,
	}

	select {
	case em.stateEvents <- event:
	default:
		em.logger.Warn("State event channel full, dropping event", "type", eventType)
	}
}

// SubscribeToApplicationEvents subscribes to application events
func (em *EventManager) SubscribeToApplicationEvents() <-chan *ApplicationEvent {
	ch := make(chan *ApplicationEvent, 10)
	em.mu.Lock()
	em.appSubscribers = append(em.appSubscribers, ch)
	em.mu.Unlock()
	return ch
}

// SubscribeToTaskEvents subscribes to task events
func (em *EventManager) SubscribeToTaskEvents() <-chan *TaskEvent {
	ch := make(chan *TaskEvent, 10)
	em.mu.Lock()
	em.taskSubscribers = append(em.taskSubscribers, ch)
	em.mu.Unlock()
	return ch
}

// SubscribeToStateEvents subscribes to state events
func (em *EventManager) SubscribeToStateEvents() <-chan *StateEvent {
	ch := make(chan *StateEvent, 10)
	em.mu.Lock()
	em.stateSubscribers = append(em.stateSubscribers, ch)
	em.mu.Unlock()
	return ch
}

// distributeApplicationEvent distributes application events to subscribers
func (em *EventManager) distributeApplicationEvent(event *ApplicationEvent) {
	em.mu.RLock()
	subscribers := make([]chan *ApplicationEvent, len(em.appSubscribers))
	copy(subscribers, em.appSubscribers)
	em.mu.RUnlock()

	for _, sub := range subscribers {
		select {
		case sub <- event:
		default:
			// Subscriber channel is full, skip
		}
	}
}

// distributeTaskEvent distributes task events to subscribers
func (em *EventManager) distributeTaskEvent(event *TaskEvent) {
	em.mu.RLock()
	subscribers := make([]chan *TaskEvent, len(em.taskSubscribers))
	copy(subscribers, em.taskSubscribers)
	em.mu.RUnlock()

	for _, sub := range subscribers {
		select {
		case sub <- event:
		default:
			// Subscriber channel is full, skip
		}
	}
}

// distributeStateEvent distributes state events to subscribers
func (em *EventManager) distributeStateEvent(event *StateEvent) {
	em.mu.RLock()
	subscribers := make([]chan *StateEvent, len(em.stateSubscribers))
	copy(subscribers, em.stateSubscribers)
	em.mu.RUnlock()

	for _, sub := range subscribers {
		select {
		case sub <- event:
		default:
			// Subscriber channel is full, skip
		}
	}
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker(log logger.Logger) *ResourceTracker {
	return &ResourceTracker{
		logger:    log.WithGroup("resources"),
		startTime: time.Now(),
		interval:  30 * time.Second,
		stopChan:  make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// Start starts the resource tracker
func (rt *ResourceTracker) Start() error {
	go rt.trackingLoop()
	return nil
}

// Stop stops the resource tracker
func (rt *ResourceTracker) Stop() {
	close(rt.stopChan)
	<-rt.done
}

// trackingLoop tracks resource usage
func (rt *ResourceTracker) trackingLoop() {
	defer close(rt.done)

	ticker := time.NewTicker(rt.interval)
	defer ticker.Stop()

	for {
		select {
		case <-rt.stopChan:
			return
		case <-ticker.C:
			rt.collectMetrics()
		}
	}
}

// collectMetrics collects resource usage metrics
func (rt *ResourceTracker) collectMetrics() {
	// This would collect actual resource metrics
	// Implementation depends on the platform and requirements
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Placeholder implementation
	rt.goroutineCount = runtime.NumGoroutine()
	rt.goroutineHistory = append(rt.goroutineHistory, rt.goroutineCount)

	// Keep only last 100 samples
	if len(rt.goroutineHistory) > 100 {
		rt.goroutineHistory = rt.goroutineHistory[1:]
	}
}
