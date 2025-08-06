# Wake - Universal Task Runner
## Product Requirements Document (PRD)

### Overview
Wake is a universal task runner wrapper that provides a unified interface for various build tools and task runners (make, npm/pnpm/yarn/bun, just, task, etc.). It features both CLI and TUI modes, offering developers a consistent way to discover, manage, and execute tasks across different project types.

### Problem Statement
Developers often work with projects that use different task runners (Makefiles, package.json scripts, Justfiles, etc.). Currently, they must:
- Remember different command syntaxes for each tool
- Navigate to specific directories to run commands
- Manually discover available tasks in each project
- Switch between different interfaces and workflows

### Solution
Wake provides a unified interface that:
- Auto-discovers task files across the project hierarchy
- Presents all available tasks in a consistent format
- Offers both CLI and interactive TUI modes
- Tracks execution time and status for all tasks
- Supports configuration and customization

---

## Core Features

### 1. Task Discovery & Parsing
**Functionality:**
- Recursively searches current and parent directories for supported task files
- Parses and extracts available commands from each file type
- Maintains metadata about each command (location, runner type, description)

**Supported Task Files:**
- `Makefile` → make
- `Justfile` → just  
- `package.json` → npm/pnpm/yarn/bun
- `Taskfile.yml` → task (Go task runner)
- `tasks.py` → custom Python task runner

### 2. Command Line Interface (CLI)

#### Basic Commands
```bash
wake                           # Launch TUI
wake <command>                 # Execute specific command
wake -l                        # List all available commands
wake --dry <command>           # Show what would be executed
```

#### Filtering Options
```bash
wake --make                    # Show only make commands
wake --just                    # Show only just commands  
wake --npm                     # Show only npm-family commands
```

#### Environment & Configuration
```bash
wake --env=<file1,file2> <cmd> # Load environment files
wake -e <file1,file2> <cmd>    # Short form of --env
```

#### Conflict Resolution
When multiple commands share the same name, the CLI prompts users to select based on:
- File path location
- Command runner type
- Project context

### 3. Terminal User Interface (TUI)

#### Layout Structure
```
┌─────────────────┬─make───────────────────────────────┐
│Search: '/'      │ echo "Running project"             │
├─────────────────┤ vite dev --host 0.0.0.0            │
│   RUNNING       ├────────────────────────────────────┤
│ > build (make)  │                                    │
│ ● test (npm)    │                                    │
│                 │                                    │
│   AVAILABLE     │                                    │
│   dev (npm)     │                                    │
│   lint (just)   │ > make build                       │
│   clean (make)  │ Building project...                │
│                 │ ✓ Build completed successfully     │
│                 │                                    │
└─────────────────┴────────────────────────────────────┘
```

#### Left Sidebar
- **Top Section:** Currently running commands with status indicators
- **Bottom Section:** Available commands grouped by status
- **Search Bar:** Filter commands with '/' key
- **Status Indicators:** Visual representation of command states

#### Right Panel
- **Top Section:** Command details and metadata
- **Bottom Section:** Real-time command output and logs

### 4. Configuration System

#### Configuration Hierarchy
1. `~/.config/wake/config.toml` (Global configuration)
2. `.wakeconfig.toml` (Project-specific, searched upward)
3. Command-line flags (Highest priority)

#### Configuration Options
```toml
[ui]
theme = "dark"              # Color scheme
layout = "split"            # TUI layout mode

[keybindings]
quit = ["q", "esc", "ctrl+c"]
run = ["space"]
search = ["/"]

[sources]
enabled = ["make", "npm", "just", "task"]
disabled = ["python"]

[hooks]
pre_command = "echo 'Starting: $CMD'"
post_command = "echo 'Finished: $CMD in $TIME'"

[runners]
npm_priority = ["pnpm", "yarn", "npm", "bun"]
```

#### Project Root Detection
- Search upward for configuration files
- If config found outside `~/.config/wake/` or `~/.wakeconfig.toml`, treat as project root
- Walk down from project root to discover task files

---

## User Interactions

### TUI Navigation & Controls

#### Global Keybindings
- `h/j/k/l` - Navigate between UI sections
- `q/esc/ctrl+c` - Exit application
- `?` - Show help menu with all keybindings

#### Sidebar Controls
- `space` - Run or stop selected command
- `enter` - Focus output section for selected command
- `/` - Open search mode
- `r` - Restart selected command
- `c` - Clear output for selected command
- `esc` - Clear search filter (when searching)

#### Output Panel Controls
- `tab` - Switch between command info and output views
- `e` - Open output in external editor
- `v` - Enter visual selection mode
- `y` - Copy selected text to clipboard
- `z` - Toggle sidebar visibility
- Mouse click - Select and copy text

### Command Execution Flow
1. User selects command (CLI or TUI)
2. Wake resolves command location and runner
3. Pre-command hooks execute (if configured)
4. Command runs with timing and output capture
5. Post-command hooks execute (if configured)
6. Results displayed with execution time and status

---

## Technical Specifications

### Implementation Language
- **Primary:** Go (Golang)
- **Rationale:** Cross-platform compatibility, excellent CLI/TUI libraries, fast execution
- **Coding Patterns.**: SOLID, KISS, Strategy so that it's easy to add new features

### Key Dependencies
- TUI Framework: Bubble Tea
- CLI Framework: Cobra
- Configuration: TOML parsing
- File system operations: Go standard library
- Process execution: based on the task runner(check if it exists or not)

### Performance Requirements
- Task discovery: < 100ms for typical projects
- TUI responsiveness: < 16ms frame updates
- Memory usage: < 50MB during normal operation
- Startup time: < 200ms

### Platform Support
- Linux (primary)
- macOS
- Windows

---

## Success Metrics

### User Experience
- Reduce task execution friction by 80%
- Support 95% of common development workflows
- Zero-configuration setup for standard project structures
- The color of the command is changed according to the status of the task

### Performance
- Sub-second task discovery across large projects
- Real-time output streaming without lag
- Minimal resource overhead during monitoring
