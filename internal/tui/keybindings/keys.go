package keybindings

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vivalchemy/wake/pkg/logger"
)

// KeyMap represents a complete key mapping for the application
type KeyMap struct {
	// Global keys
	Quit    key.Binding
	Help    key.Binding
	Refresh key.Binding

	// Navigation
	Up       key.Binding
	Down     key.Binding
	Left     key.Binding
	Right    key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Home     key.Binding
	End      key.Binding
	Tab      key.Binding
	ShiftTab key.Binding

	// Actions
	Enter     key.Binding
	Escape    key.Binding
	Space     key.Binding
	Delete    key.Binding
	Backspace key.Binding

	// Task operations
	RunTask    key.Binding
	StopTask   key.Binding
	DryRunTask key.Binding
	SelectTask key.Binding

	// View operations
	ToggleSearch key.Binding
	ToggleHelp   key.Binding
	ToggleHidden key.Binding
	CycleView    key.Binding
	SplitView    key.Binding

	// Filtering and sorting
	FilterRunner key.Binding
	FilterStatus key.Binding
	FilterTags   key.Binding
	SortByName   key.Binding
	SortByRunner key.Binding
	ClearFilter  key.Binding

	// Layout
	ResizeLeft  key.Binding
	ResizeRight key.Binding
	ResizeUp    key.Binding
	ResizeDown  key.Binding
	ToggleFocus key.Binding

	// Output operations
	ClearOutput  key.Binding
	SaveOutput   key.Binding
	CopyOutput   key.Binding
	ScrollOutput key.Binding

	// Application modes
	TUIMode         key.Binding
	InteractiveMode key.Binding
}

// KeyContext represents different contexts where keys can be active
type KeyContext string

const (
	ContextGlobal  KeyContext = "global"
	ContextSidebar KeyContext = "sidebar"
	ContextOutput  KeyContext = "output"
	ContextSearch  KeyContext = "search"
	ContextHelp    KeyContext = "help"
	ContextDialog  KeyContext = "dialog"
)

// KeyBinding represents a single key binding with context
type KeyBinding struct {
	Key         key.Binding
	Context     KeyContext
	Description string
	Action      KeyAction
	Enabled     bool
}

// KeyAction represents an action that can be triggered by a key
type KeyAction func() tea.Cmd

// KeyManager manages key bindings and contexts
type KeyManager struct {
	// Key mappings
	keyMap   *KeyMap
	bindings map[KeyContext][]KeyBinding

	// State
	currentContext KeyContext
	enabled        bool

	// Configuration
	customBindings map[string]key.Binding

	// Logger
	logger logger.Logger
}

// KeyHelpSection represents a section in the help display
type KeyHelpSection struct {
	Title string
	Keys  []key.Binding
}

// KeyProfile represents a complete set of key bindings
type KeyProfile struct {
	Name        string
	Description string
	KeyMap      *KeyMap
}

// Predefined key profiles
var (
	// Default key profile
	DefaultProfile = &KeyProfile{
		Name:        "default",
		Description: "Default key bindings",
		KeyMap:      createDefaultKeyMap(),
	}

	// Vim-like key profile
	VimProfile = &KeyProfile{
		Name:        "vim",
		Description: "Vim-like key bindings",
		KeyMap:      createVimKeyMap(),
	}

	// Emacs-like key profile
	EmacsProfile = &KeyProfile{
		Name:        "emacs",
		Description: "Emacs-like key bindings",
		KeyMap:      createEmacsKeyMap(),
	}
)

// NewKeyManager creates a new key manager
func NewKeyManager(logger logger.Logger) *KeyManager {
	km := &KeyManager{
		keyMap:         createDefaultKeyMap(),
		bindings:       make(map[KeyContext][]KeyBinding),
		currentContext: ContextGlobal,
		enabled:        true,
		customBindings: make(map[string]key.Binding),
		logger:         logger.WithGroup("keybindings"),
	}

	// Initialize default bindings
	km.initializeDefaultBindings()

	return km
}

// initializeDefaultBindings sets up the default key bindings
func (km *KeyManager) initializeDefaultBindings() {
	// Global bindings
	km.AddBinding(ContextGlobal, KeyBinding{
		Key:         km.keyMap.Quit,
		Context:     ContextGlobal,
		Description: "Quit application",
		Enabled:     true,
	})

	km.AddBinding(ContextGlobal, KeyBinding{
		Key:         km.keyMap.Help,
		Context:     ContextGlobal,
		Description: "Show help",
		Enabled:     true,
	})

	km.AddBinding(ContextGlobal, KeyBinding{
		Key:         km.keyMap.Refresh,
		Context:     ContextGlobal,
		Description: "Refresh tasks",
		Enabled:     true,
	})

	// Sidebar bindings
	km.AddBinding(ContextSidebar, KeyBinding{
		Key:         km.keyMap.Up,
		Context:     ContextSidebar,
		Description: "Move up",
		Enabled:     true,
	})

	km.AddBinding(ContextSidebar, KeyBinding{
		Key:         km.keyMap.Down,
		Context:     ContextSidebar,
		Description: "Move down",
		Enabled:     true,
	})

	km.AddBinding(ContextSidebar, KeyBinding{
		Key:         km.keyMap.Enter,
		Context:     ContextSidebar,
		Description: "Select task",
		Enabled:     true,
	})

	km.AddBinding(ContextSidebar, KeyBinding{
		Key:         km.keyMap.RunTask,
		Context:     ContextSidebar,
		Description: "Run task",
		Enabled:     true,
	})

	// Output bindings
	km.AddBinding(ContextOutput, KeyBinding{
		Key:         km.keyMap.ClearOutput,
		Context:     ContextOutput,
		Description: "Clear output",
		Enabled:     true,
	})

	km.AddBinding(ContextOutput, KeyBinding{
		Key:         km.keyMap.ScrollOutput,
		Context:     ContextOutput,
		Description: "Toggle auto-scroll",
		Enabled:     true,
	})

	// Search bindings
	km.AddBinding(ContextSearch, KeyBinding{
		Key:         km.keyMap.Escape,
		Context:     ContextSearch,
		Description: "Close search",
		Enabled:     true,
	})

	km.logger.Info("Initialized default key bindings")
}

// SetKeyMap sets the active key map
func (km *KeyManager) SetKeyMap(keyMap *KeyMap) {
	km.keyMap = keyMap
	km.initializeDefaultBindings() // Reinitialize with new keymap
}

// SetProfile sets the active key profile
func (km *KeyManager) SetProfile(profileName string) error {
	var profile *KeyProfile

	switch profileName {
	case "default":
		profile = DefaultProfile
	case "vim":
		profile = VimProfile
	case "emacs":
		profile = EmacsProfile
	default:
		return fmt.Errorf("unknown key profile: %s", profileName)
	}

	km.SetKeyMap(profile.KeyMap)
	km.logger.Info("Set key profile", "profile", profileName)

	return nil
}

// GetKeyMap returns the current key map
func (km *KeyManager) GetKeyMap() *KeyMap {
	return km.keyMap
}

// SetContext sets the current key context
func (km *KeyManager) SetContext(context KeyContext) {
	km.currentContext = context
	km.logger.Debug("Changed key context", "context", string(context))
}

// GetContext returns the current key context
func (km *KeyManager) GetContext() KeyContext {
	return km.currentContext
}

// AddBinding adds a key binding to a context
func (km *KeyManager) AddBinding(context KeyContext, binding KeyBinding) {
	km.bindings[context] = append(km.bindings[context], binding)
}

// RemoveBinding removes a key binding from a context
func (km *KeyManager) RemoveBinding(context KeyContext, keyStr string) {
	bindings := km.bindings[context]
	for i, binding := range bindings {
		if binding.Key.Keys()[0] == keyStr {
			km.bindings[context] = append(bindings[:i], bindings[i+1:]...)
			break
		}
	}
}

// GetBindings returns all bindings for a context
func (km *KeyManager) GetBindings(context KeyContext) []KeyBinding {
	return km.bindings[context]
}

// GetActiveBindings returns bindings for the current context plus global bindings
func (km *KeyManager) GetActiveBindings() []KeyBinding {
	var activeBindings []KeyBinding

	// Add global bindings
	activeBindings = append(activeBindings, km.bindings[ContextGlobal]...)

	// Add context-specific bindings
	if km.currentContext != ContextGlobal {
		activeBindings = append(activeBindings, km.bindings[km.currentContext]...)
	}

	return activeBindings
}

// HandleKey handles a key press and returns the appropriate command
func (km *KeyManager) HandleKey(msg tea.KeyMsg) tea.Cmd {
	if !km.enabled {
		return nil
	}

	activeBindings := km.GetActiveBindings()

	for _, binding := range activeBindings {
		if binding.Enabled && key.Matches(msg, binding.Key) {
			if binding.Action != nil {
				km.logger.Debug("Key binding triggered",
					"key", binding.Key.Keys(),
					"context", string(binding.Context),
					"description", binding.Description)
				return binding.Action()
			}
		}
	}

	return nil
}

// SetEnabled enables or disables key handling
func (km *KeyManager) SetEnabled(enabled bool) {
	km.enabled = enabled
}

// IsEnabled returns whether key handling is enabled
func (km *KeyManager) IsEnabled() bool {
	return km.enabled
}

// GetHelpSections returns help sections for the current context
func (km *KeyManager) GetHelpSections() []KeyHelpSection {
	var sections []KeyHelpSection

	// Global section
	globalKeys := make([]key.Binding, 0)
	for _, binding := range km.bindings[ContextGlobal] {
		if binding.Enabled {
			globalKeys = append(globalKeys, binding.Key)
		}
	}
	if len(globalKeys) > 0 {
		sections = append(sections, KeyHelpSection{
			Title: "Global",
			Keys:  globalKeys,
		})
	}

	// Context-specific section
	if km.currentContext != ContextGlobal {
		contextKeys := make([]key.Binding, 0)
		for _, binding := range km.bindings[km.currentContext] {
			if binding.Enabled {
				contextKeys = append(contextKeys, binding.Key)
			}
		}
		if len(contextKeys) > 0 {
			sections = append(sections, KeyHelpSection{
				Title: strings.Title(string(km.currentContext)),
				Keys:  contextKeys,
			})
		}
	}

	return sections
}

// CustomizeKey allows customizing a key binding
func (km *KeyManager) CustomizeKey(name string, keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("at least one key must be specified")
	}

	binding := key.NewBinding(
		key.WithKeys(keys...),
		key.WithHelp(keys[0], name),
	)

	km.customBindings[name] = binding
	km.logger.Info("Customized key binding", "name", name, "keys", keys)

	return nil
}

// GetCustomKey returns a custom key binding
func (km *KeyManager) GetCustomKey(name string) (key.Binding, bool) {
	binding, exists := km.customBindings[name]
	return binding, exists
}

// Export exports key bindings to a map for serialization
func (km *KeyManager) Export() map[string]any {
	export := make(map[string]any)

	// Export context bindings
	for context, bindings := range km.bindings {
		contextMap := make([]map[string]any, 0)
		for _, binding := range bindings {
			bindingMap := map[string]any{
				"keys":        binding.Key.Keys(),
				"description": binding.Description,
				"enabled":     binding.Enabled,
			}
			contextMap = append(contextMap, bindingMap)
		}
		export[string(context)] = contextMap
	}

	// Export custom bindings
	customMap := make(map[string][]string)
	for name, binding := range km.customBindings {
		customMap[name] = binding.Keys()
	}
	export["custom"] = customMap

	return export
}

// Import imports key bindings from a map
func (km *KeyManager) Import(data map[string]any) error {
	// Import custom bindings
	if customData, exists := data["custom"]; exists {
		if customMap, ok := customData.(map[string]any); ok {
			for name, keysData := range customMap {
				if keysList, ok := keysData.([]any); ok {
					keys := make([]string, len(keysList))
					for i, keyData := range keysList {
						if keyStr, ok := keyData.(string); ok {
							keys[i] = keyStr
						}
					}
					km.CustomizeKey(name, keys)
				}
			}
		}
	}

	return nil
}

// Default key map creation
func createDefaultKeyMap() *KeyMap {
	return &KeyMap{
		// Global keys
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c", "q"),
			key.WithHelp("ctrl+c/q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?", "f1"),
			key.WithHelp("?", "help"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r", "f5"),
			key.WithHelp("ctrl+r", "refresh"),
		),

		// Navigation
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdown", "page down"),
		),
		Home: key.NewBinding(
			key.WithKeys("home", "g"),
			key.WithHelp("home/g", "go to start"),
		),
		End: key.NewBinding(
			key.WithKeys("end", "G"),
			key.WithHelp("end/G", "go to end"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "previous"),
		),

		// Actions
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Space: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle"),
		),
		Delete: key.NewBinding(
			key.WithKeys("delete", "x"),
			key.WithHelp("del/x", "delete"),
		),
		Backspace: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("backspace", "back"),
		),

		// Task operations
		RunTask: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "run task"),
		),
		StopTask: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "stop task"),
		),
		DryRunTask: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "dry run"),
		),
		SelectTask: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select task"),
		),

		// View operations
		ToggleSearch: key.NewBinding(
			key.WithKeys("/", "ctrl+f"),
			key.WithHelp("/", "search"),
		),
		ToggleHelp: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		ToggleHidden: key.NewBinding(
			key.WithKeys("ctrl+h"),
			key.WithHelp("ctrl+h", "toggle hidden"),
		),
		CycleView: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "cycle view"),
		),
		SplitView: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "split view"),
		),

		// Filtering and sorting
		FilterRunner: key.NewBinding(
			key.WithKeys("ctrl+shift+r"),
			key.WithHelp("ctrl+shift+r", "filter by runner"),
		),
		FilterStatus: key.NewBinding(
			key.WithKeys("ctrl+shift+s"),
			key.WithHelp("ctrl+shift+s", "filter by status"),
		),
		FilterTags: key.NewBinding(
			key.WithKeys("ctrl+shift+t"),
			key.WithHelp("ctrl+shift+t", "filter by tags"),
		),
		SortByName: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "sort by name"),
		),
		SortByRunner: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("ctrl+n", "sort by runner"),
		),
		ClearFilter: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clear filters"),
		),

		// Layout
		ResizeLeft: key.NewBinding(
			key.WithKeys("ctrl+left"),
			key.WithHelp("ctrl+←", "resize left"),
		),
		ResizeRight: key.NewBinding(
			key.WithKeys("ctrl+right"),
			key.WithHelp("ctrl+→", "resize right"),
		),
		ResizeUp: key.NewBinding(
			key.WithKeys("ctrl+up"),
			key.WithHelp("ctrl+↑", "resize up"),
		),
		ResizeDown: key.NewBinding(
			key.WithKeys("ctrl+down"),
			key.WithHelp("ctrl+↓", "resize down"),
		),
		ToggleFocus: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "toggle focus"),
		),

		// Output operations
		ClearOutput: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear output"),
		),
		SaveOutput: key.NewBinding(
			key.WithKeys("ctrl+shift+s"),
			key.WithHelp("ctrl+shift+s", "save output"),
		),
		CopyOutput: key.NewBinding(
			key.WithKeys("ctrl+shift+c"),
			key.WithHelp("ctrl+shift+c", "copy output"),
		),
		ScrollOutput: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "toggle auto-scroll"),
		),

		// Application modes
		TUIMode: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "TUI mode"),
		),
		InteractiveMode: key.NewBinding(
			key.WithKeys("ctrl+i"),
			key.WithHelp("ctrl+i", "interactive mode"),
		),
	}
}

// Vim key map creation
func createVimKeyMap() *KeyMap {
	keyMap := createDefaultKeyMap()

	// Override with vim-like bindings
	keyMap.Up = key.NewBinding(
		key.WithKeys("k"),
		key.WithHelp("k", "up"),
	)
	keyMap.Down = key.NewBinding(
		key.WithKeys("j"),
		key.WithHelp("j", "down"),
	)
	keyMap.Left = key.NewBinding(
		key.WithKeys("h"),
		key.WithHelp("h", "left"),
	)
	keyMap.Right = key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "right"),
	)
	keyMap.Home = key.NewBinding(
		key.WithKeys("gg"),
		key.WithHelp("gg", "go to start"),
	)
	keyMap.End = key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to end"),
	)
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", ":q"),
		key.WithHelp(":q", "quit"),
	)

	return keyMap
}

// Emacs key map creation
func createEmacsKeyMap() *KeyMap {
	keyMap := createDefaultKeyMap()

	// Override with emacs-like bindings
	keyMap.Up = key.NewBinding(
		key.WithKeys("ctrl+p"),
		key.WithHelp("ctrl+p", "up"),
	)
	keyMap.Down = key.NewBinding(
		key.WithKeys("ctrl+n"),
		key.WithHelp("ctrl+n", "down"),
	)
	keyMap.Left = key.NewBinding(
		key.WithKeys("ctrl+b"),
		key.WithHelp("ctrl+b", "left"),
	)
	keyMap.Right = key.NewBinding(
		key.WithKeys("ctrl+f"),
		key.WithHelp("ctrl+f", "right"),
	)
	keyMap.Home = key.NewBinding(
		key.WithKeys("ctrl+a"),
		key.WithHelp("ctrl+a", "go to start"),
	)
	keyMap.End = key.NewBinding(
		key.WithKeys("ctrl+e"),
		key.WithHelp("ctrl+e", "go to end"),
	)
	keyMap.Quit = key.NewBinding(
		key.WithKeys("ctrl+x", "ctrl+c"),
		key.WithHelp("ctrl+x ctrl+c", "quit"),
	)

	return keyMap
}

// GetAvailableProfiles returns available key profiles
func GetAvailableProfiles() []string {
	return []string{"default", "vim", "emacs"}
}

// GetProfileInfo returns information about a key profile
func GetProfileInfo(profileName string) (*KeyProfile, error) {
	switch profileName {
	case "default":
		return DefaultProfile, nil
	case "vim":
		return VimProfile, nil
	case "emacs":
		return EmacsProfile, nil
	default:
		return nil, fmt.Errorf("unknown profile: %s", profileName)
	}
}
