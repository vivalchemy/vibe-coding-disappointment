package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vivalchemy/wake/internal/app"
	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/internal/discovery"
	"github.com/vivalchemy/wake/internal/runner"
	"github.com/vivalchemy/wake/internal/tui/components/layout"
	"github.com/vivalchemy/wake/internal/tui/keybindings"
	"github.com/vivalchemy/wake/internal/tui/theme"
	"github.com/vivalchemy/wake/pkg/logger"
)

// TUI represents the main TUI application
type TUI struct {
	// Core components
	model   *Model
	program *tea.Program

	// Application context
	appContext *app.Context

	// Configuration
	config *config.Config

	// Core services
	discovery *discovery.Registry
	executor  *runner.Executor

	// UI components
	layout        *layout.Layout
	keyManager    *keybindings.KeyManager
	themeProvider *theme.ThemeProvider

	// State
	initialized bool
	running     bool

	// Logger
	logger logger.Logger
}

// Options for TUI configuration
type Options struct {
	Config       *config.Config
	Discovery    *discovery.Registry
	Executor     *runner.Executor
	Logger       logger.Logger
	AppContext   *app.Context
	Theme        string
	KeyProfile   string
	AltScreen    bool
	MouseEnabled bool
}

// New creates a new TUI application
func New(opts Options) (*TUI, error) {
	if opts.Config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}
	if opts.AppContext == nil {
		return nil, fmt.Errorf("app context is required")
	}

	// Create theme provider
	themeProvider := theme.NewThemeProvider(opts.Logger)
	if opts.Theme != "" {
		if err := themeProvider.SetTheme(opts.Theme); err != nil {
			opts.Logger.Warn("Failed to set theme, using default", "theme", opts.Theme, "error", err)
		}
	}

	// Create key manager
	keyManager := keybindings.NewKeyManager(opts.Logger)
	if opts.KeyProfile != "" {
		if err := keyManager.SetProfile(opts.KeyProfile); err != nil {
			opts.Logger.Warn("Failed to set key profile, using default", "profile", opts.KeyProfile, "error", err)
		}
	}

	// Create layout
	layoutComponent, err := layout.New(opts.Logger, themeProvider.GetTheme())
	if err != nil {
		return nil, fmt.Errorf("failed to create layout: %w", err)
	}

	// Create model
	model := NewModel(opts.Config, opts.Logger)

	tui := &TUI{
		model:         model,
		appContext:    opts.AppContext,
		config:        opts.Config,
		discovery:     opts.Discovery,
		executor:      opts.Executor,
		layout:        layoutComponent,
		keyManager:    keyManager,
		themeProvider: themeProvider,
		initialized:   false,
		running:       false,
		logger:        opts.Logger.WithGroup("tui"),
	}

	// Initialize model with components
	model.SetComponents(tui.layout, tui.keyManager, tui.themeProvider)

	// Create tea program
	programOpts := []tea.ProgramOption{
		tea.WithAltScreen(),
	}

	if opts.MouseEnabled {
		programOpts = append(programOpts, tea.WithMouseCellMotion())
	}

	if !opts.AltScreen {
		programOpts = programOpts[:0] // Remove alt screen if disabled
	}

	tui.program = tea.NewProgram(model, programOpts...)

	return tui, nil
}

// Initialize initializes the TUI
func (t *TUI) Initialize() error {
	if t.initialized {
		return nil
	}

	t.logger.Info("Initializing TUI")

	// Set application mode to TUI
	t.appContext.SetMode(app.ModeTUI)

	// Initialize components
	if err := t.initializeComponents(); err != nil {
		return fmt.Errorf("failed to initialize components: %w", err)
	}

	// Setup event handling
	t.setupEventHandling()

	// Load initial tasks if discovery is available
	if t.discovery != nil {
		go t.loadInitialTasks()
	}

	t.initialized = true
	t.logger.Info("TUI initialized successfully")

	return nil
}

// Run starts the TUI application
func (t *TUI) Run() error {
	if !t.initialized {
		if err := t.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize TUI: %w", err)
		}
	}

	t.logger.Info("Starting TUI application")
	t.running = true

	// Start the program
	model, err := t.program.Run()
	if err != nil {
		t.running = false
		return fmt.Errorf("TUI program error: %w", err)
	}

	// Process final model state
	if finalModel, ok := model.(*Model); ok {
		t.handleFinalState(finalModel)
	}

	t.running = false
	t.logger.Info("TUI application stopped")

	return nil
}

// Stop stops the TUI application
func (t *TUI) Stop() error {
	if !t.running {
		return nil
	}

	t.logger.Info("Stopping TUI application")

	// Send quit message to program
	t.program.Send(QuitMsg{})

	// Wait for program to stop gracefully
	time.Sleep(100 * time.Millisecond)

	// Kill program if still running
	t.program.Kill()

	return nil
}

// IsRunning returns whether the TUI is currently running
func (t *TUI) IsRunning() bool {
	return t.running
}

// GetThemeProvider returns the theme provider
func (t *TUI) GetThemeProvider() *theme.ThemeProvider {
	return t.themeProvider
}

// GetKeyManager returns the key manager
func (t *TUI) GetKeyManager() *keybindings.KeyManager {
	return t.keyManager
}

// GetLayout returns the layout component
func (t *TUI) GetLayout() *layout.Layout {
	return t.layout
}

// SendMessage sends a message to the TUI
func (t *TUI) SendMessage(msg tea.Msg) {
	if t.program != nil {
		t.program.Send(msg)
	}
}

// initializeComponents initializes all TUI components
func (t *TUI) initializeComponents() error {
	// Initialize layout
	if err := t.initializeLayout(); err != nil {
		return fmt.Errorf("failed to initialize layout: %w", err)
	}

	// Initialize key bindings
	t.initializeKeyBindings()

	// Initialize theme
	t.initializeTheme()

	return nil
}

// initializeLayout initializes the layout component
func (t *TUI) initializeLayout() error {
	// Set initial focus
	t.layout.SetFocusedComponent(layout.FocusSidebar)

	// Configure layout based on config
	if t.config.UI != nil {
		if t.config.UI.SplitMode != "" {
			switch strings.ToLower(t.config.UI.SplitMode) {
			case "vertical":
				t.layout.SetSplitMode(layout.SplitVertical)
			case "horizontal":
				t.layout.SetSplitMode(layout.SplitHorizontal)
			case "dynamic":
				t.layout.SetSplitMode(layout.SplitDynamic)
			}
		}
	}

	return nil
}

// initializeKeyBindings initializes key bindings and actions
func (t *TUI) initializeKeyBindings() {
	keyMap := t.keyManager.GetKeyMap()

	// Setup global key actions
	t.setupGlobalKeyActions(keyMap)

	// Setup context-specific key actions
	t.setupContextKeyActions(keyMap)
}

// setupGlobalKeyActions sets up global key actions
func (t *TUI) setupGlobalKeyActions(keyMap *keybindings.KeyMap) {
	// These would be implemented to send appropriate messages
	// For brevity, showing the structure

	// Quit action
	t.keyManager.AddBinding(keybindings.ContextGlobal, keybindings.KeyBinding{
		Key:         keyMap.Quit,
		Context:     keybindings.ContextGlobal,
		Description: "Quit application",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return QuitMsg{}
			}
		},
		Enabled: true,
	})

	// Help action
	t.keyManager.AddBinding(keybindings.ContextGlobal, keybindings.KeyBinding{
		Key:         keyMap.Help,
		Context:     keybindings.ContextGlobal,
		Description: "Show help",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return ShowHelpMsg{}
			}
		},
		Enabled: true,
	})

	// Refresh action
	t.keyManager.AddBinding(keybindings.ContextGlobal, keybindings.KeyBinding{
		Key:         keyMap.Refresh,
		Context:     keybindings.ContextGlobal,
		Description: "Refresh tasks",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return RefreshTasksMsg{}
			}
		},
		Enabled: true,
	})
}

// setupContextKeyActions sets up context-specific key actions
func (t *TUI) setupContextKeyActions(keyMap *keybindings.KeyMap) {
	// Sidebar actions
	t.keyManager.AddBinding(keybindings.ContextSidebar, keybindings.KeyBinding{
		Key:         keyMap.RunTask,
		Context:     keybindings.ContextSidebar,
		Description: "Run selected task",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return RunSelectedTaskMsg{}
			}
		},
		Enabled: true,
	})

	// Search actions
	t.keyManager.AddBinding(keybindings.ContextGlobal, keybindings.KeyBinding{
		Key:         keyMap.ToggleSearch,
		Context:     keybindings.ContextGlobal,
		Description: "Toggle search",
		Action: func() tea.Cmd {
			return func() tea.Msg {
				return ToggleSearchMsg{}
			}
		},
		Enabled: true,
	})
}

// initializeTheme initializes the theme
func (t *TUI) initializeTheme() {
	// Adapt theme to terminal capabilities
	t.themeProvider.AdaptToTerminal()

	// Apply theme to components
	currentTheme := t.themeProvider.GetTheme()
	if currentTheme != nil {
		t.logger.Info("Applied theme", "theme", currentTheme.Name())
	}
}

// setupEventHandling sets up event handling between components
func (t *TUI) setupEventHandling() {
	// Subscribe to application events
	appEvents := t.appContext.SubscribeToApplicationEvents()
	go t.handleApplicationEvents(appEvents)

	// Subscribe to state events
	stateEvents := t.appContext.SubscribeToStateEvents()
	go t.handleStateEvents(stateEvents)
}

// handleApplicationEvents handles application-level events
func (t *TUI) handleApplicationEvents(events <-chan *app.ApplicationEvent) {
	for event := range events {
		switch event.Type {
		case app.EventAppError:
			t.SendMessage(ErrorMsg{Error: fmt.Errorf("application error: %v", event.Error)})

		case app.EventDiscoveryStarted:
			t.SendMessage(DiscoveryStartedMsg{})

		case app.EventDiscoveryCompleted:
			t.SendMessage(DiscoveryCompletedMsg{})

		case app.EventConfigChanged:
			t.SendMessage(ConfigChangedMsg{})
		}
	}
}

// handleStateEvents handles application state events
func (t *TUI) handleStateEvents(events <-chan *app.StateEvent) {
	for event := range events {
		switch event.Type {
		case app.StateEventTaskCountChanged:
			if newCount, ok := event.NewValue.(int); ok {
				t.SendMessage(TaskCountChangedMsg{Count: newCount})
			}

		case app.StateEventErrorOccurred:
			if err, ok := event.NewValue.(error); ok {
				t.SendMessage(ErrorMsg{Error: err})
			}
		}
	}
}

// loadInitialTasks loads initial tasks from discovery
func (t *TUI) loadInitialTasks() {
	t.logger.Debug("Loading initial tasks")

	// Discover tasks
	tasks, err := t.discovery.DiscoverTasks(context.Background(), ".")
	if err != nil {
		t.SendMessage(ErrorMsg{Error: fmt.Errorf("failed to discover tasks: %w", err)})
		return
	}

	t.logger.Info("Discovered tasks", "count", len(tasks))

	// Send tasks to UI
	t.SendMessage(TasksDiscoveredMsg{Tasks: tasks})
}

// handleFinalState processes the final model state
func (t *TUI) handleFinalState(model *Model) {
	state := model.GetState()

	// Log final statistics
	t.logger.Info("TUI session completed",
		"duration", state.SessionDuration,
		"tasks_run", state.TasksRun,
		"errors", state.ErrorCount)

	// Save any persistent state
	if err := t.savePersistentState(state); err != nil {
		t.logger.Warn("Failed to save persistent state", "error", err)
	}
}

// savePersistentState saves persistent state to disk
func (t *TUI) savePersistentState(state *ModelState) error {
	// Save theme preference
	if state.CurrentTheme != "" {
		// This would save to config file or user preferences
		t.logger.Debug("Saving theme preference", "theme", state.CurrentTheme)
	}

	// Save key profile preference
	if state.KeyProfile != "" {
		t.logger.Debug("Saving key profile preference", "profile", state.KeyProfile)
	}

	// Save layout preferences
	t.logger.Debug("Saving layout preferences",
		"split_mode", state.SplitMode,
		"sidebar_width", state.SidebarWidth)

	return nil
}

// GetVersion returns the TUI version information
func (t *TUI) GetVersion() string {
	return "Wake TUI v1.0.0"
}

// GetStatus returns the current TUI status
func (t *TUI) GetStatus() *TUIStatus {
	return &TUIStatus{
		Running:      t.running,
		Initialized:  t.initialized,
		Theme:        t.themeProvider.GetTheme().Name(),
		KeyProfile:   "default", // Would get from key manager
		TerminalInfo: t.themeProvider.GetTerminalInfo(),
	}
}

// TUIStatus represents the current status of the TUI
type TUIStatus struct {
	Running      bool                `json:"running"`
	Initialized  bool                `json:"initialized"`
	Theme        string              `json:"theme"`
	KeyProfile   string              `json:"key_profile"`
	TerminalInfo *theme.TerminalInfo `json:"terminal_info"`
}

// SetTheme changes the current theme
func (t *TUI) SetTheme(themeName string) error {
	if err := t.themeProvider.SetTheme(themeName); err != nil {
		return err
	}

	// Send theme change message to UI
	t.SendMessage(ThemeChangedMsg{Theme: themeName})

	return nil
}

// SetKeyProfile changes the key profile
func (t *TUI) SetKeyProfile(profileName string) error {
	if err := t.keyManager.SetProfile(profileName); err != nil {
		return err
	}

	// Send key profile change message to UI
	t.SendMessage(KeyProfileChangedMsg{Profile: profileName})

	return nil
}

// RunTask runs a specific task
func (t *TUI) RunTask(taskID string) error {
	if t.executor == nil {
		return fmt.Errorf("task executor not available")
	}

	// Send run task message to UI
	t.SendMessage(RunTaskMsg{TaskID: taskID})

	return nil
}

// GetRunningTasks returns currently running tasks
func (t *TUI) GetRunningTasks() map[string]*runner.Execution {
	if t.executor == nil {
		return nil
	}

	return t.executor.GetActiveExecutions()
}

// EnableDebugMode enables debug mode for the TUI
func (t *TUI) EnableDebugMode() {
	t.SendMessage(EnableDebugMsg{})
}

// DisableDebugMode disables debug mode for the TUI
func (t *TUI) DisableDebugMode() {
	t.SendMessage(DisableDebugMsg{})
}

// ExportConfig exports the current TUI configuration
func (t *TUI) ExportConfig() map[string]any {
	return map[string]any{
		"theme":       t.themeProvider.GetTheme().Name(),
		"key_profile": "default", // Would get from key manager
		"layout": map[string]any{
			"split_mode": t.layout.GetSplitMode().String(),
		},
		"keybindings": t.keyManager.Export(),
	}
}

// ImportConfig imports TUI configuration
func (t *TUI) ImportConfig(config map[string]any) error {
	// Import theme
	if theme, ok := config["theme"].(string); ok {
		if err := t.SetTheme(theme); err != nil {
			t.logger.Warn("Failed to import theme", "theme", theme, "error", err)
		}
	}

	// Import key profile
	if profile, ok := config["key_profile"].(string); ok {
		if err := t.SetKeyProfile(profile); err != nil {
			t.logger.Warn("Failed to import key profile", "profile", profile, "error", err)
		}
	}

	// Import keybindings
	if keybindings, ok := config["keybindings"].(map[string]any); ok {
		if err := t.keyManager.Import(keybindings); err != nil {
			t.logger.Warn("Failed to import keybindings", "error", err)
		}
	}

	return nil
}

// Utility functions

// centerText centers text within a given width
func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}

	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text
}

// truncateText truncates text to fit within a given width
func truncateText(text string, width int) string {
	if len(text) <= width {
		return text
	}

	if width <= 3 {
		return text[:width]
	}

	return text[:width-3] + "..."
}

// formatDuration formats a duration for display
func formatDuration(duration time.Duration) string {
	if duration < time.Second {
		return fmt.Sprintf("%dms", duration.Milliseconds())
	} else if duration < time.Minute {
		return fmt.Sprintf("%.1fs", duration.Seconds())
	} else if duration < time.Hour {
		return fmt.Sprintf("%.1fm", duration.Minutes())
	} else {
		return fmt.Sprintf("%.1fh", duration.Hours())
	}
}

// wrapText wraps text to fit within a given width
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		if currentLine.Len() == 0 {
			currentLine.WriteString(word)
		} else if currentLine.Len()+1+len(word) <= width {
			currentLine.WriteString(" " + word)
		} else {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(word)
		}
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}
