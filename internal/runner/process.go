package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
)

// Process represents a running task process
type Process struct {
	// Process information
	cmd      *exec.Cmd
	pid      int
	started  bool
	finished bool

	// I/O handling
	stdout io.ReadCloser
	stderr io.ReadCloser
	stdin  io.WriteCloser

	// Output channels
	outputChan chan<- *OutputEvent

	// Process state
	exitCode int
	error    error

	// Timing
	startTime time.Time
	endTime   *time.Time

	// Control and synchronization
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.RWMutex
	logger logger.Logger

	// Resource monitoring
	resourceMonitor *ResourceMonitor

	// Configuration
	killTimeout time.Duration
}

// ProcessConfig configures process creation
type ProcessConfig struct {
	// Command and arguments
	Command string
	Args    []string

	// Environment
	Env []string

	// Working directory
	WorkingDir string

	// I/O configuration
	OutputChan chan<- *OutputEvent
	TaskID     string

	// Process options
	KillTimeout time.Duration

	// Logger
	Logger logger.Logger
}

// ResourceMonitor tracks process resource usage
type ResourceMonitor struct {
	process  *Process
	logger   logger.Logger
	interval time.Duration

	// Resource data
	maxMemory    uint64
	maxCPU       float64
	totalCPUTime time.Duration

	// Control
	stopChan chan struct{}
	done     chan struct{}
	mu       sync.RWMutex
}

// ResourceUsage represents current resource usage
type ResourceUsage struct {
	PID         int           `json:"pid"`
	MemoryRSS   uint64        `json:"memory_rss"`   // Resident Set Size in bytes
	MemoryVMS   uint64        `json:"memory_vms"`   // Virtual Memory Size in bytes
	CPUPercent  float64       `json:"cpu_percent"`  // CPU usage percentage
	CPUTime     time.Duration `json:"cpu_time"`     // Total CPU time
	NumThreads  int           `json:"num_threads"`  // Number of threads
	NumFDs      int           `json:"num_fds"`      // Number of file descriptors
	StartTime   time.Time     `json:"start_time"`   // Process start time
	RunningTime time.Duration `json:"running_time"` // How long process has been running
}

// NewProcess creates a new process from configuration
func NewProcess(ctx context.Context, config *ProcessConfig) (*Process, error) {
	if config.Command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	// Create command
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)

	// Set working directory
	if config.WorkingDir != "" {
		cmd.Dir = config.WorkingDir
	}

	// Set environment
	if len(config.Env) > 0 {
		cmd.Env = config.Env
	} else {
		cmd.Env = os.Environ()
	}

	// Create process context
	processCtx, cancel := context.WithCancel(ctx)

	// Set default kill timeout
	killTimeout := config.KillTimeout
	if killTimeout == 0 {
		killTimeout = 10 * time.Second
	}

	process := &Process{
		cmd:         cmd,
		outputChan:  config.OutputChan,
		ctx:         processCtx,
		cancel:      cancel,
		done:        make(chan struct{}),
		logger:      config.Logger.WithGroup("process"),
		killTimeout: killTimeout,
	}

	// Setup I/O
	if err := process.setupIO(); err != nil {
		return nil, fmt.Errorf("failed to setup I/O: %w", err)
	}

	// Create resource monitor if monitoring is enabled
	if config.Logger != nil {
		process.resourceMonitor = NewResourceMonitor(process, config.Logger)
	}

	return process, nil
}

// setupIO sets up stdin, stdout, and stderr pipes
func (p *Process) setupIO() error {
	var err error

	// Setup stdout pipe
	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Setup stderr pipe
	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Setup stdin pipe
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	return nil
}

// Start starts the process
func (p *Process) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return fmt.Errorf("process already started")
	}

	p.logger.Debug("Starting process", "command", p.cmd.Path, "args", p.cmd.Args)

	// Start the process
	p.startTime = time.Now()
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	p.started = true
	p.pid = p.cmd.Process.Pid

	p.logger.Info("Process started", "pid", p.pid, "command", p.cmd.Path)

	// Start output monitoring
	go p.monitorOutput()

	// Start resource monitoring
	if p.resourceMonitor != nil {
		go p.resourceMonitor.Start()
	}

	return nil
}

// Wait waits for the process to complete
func (p *Process) Wait() error {
	if !p.started {
		return fmt.Errorf("process not started")
	}

	// Wait for the process to complete
	err := p.cmd.Wait()
	endTime := time.Now()

	p.mu.Lock()
	p.finished = true
	p.endTime = &endTime

	if err != nil {
		p.error = err
		if exitError, ok := err.(*exec.ExitError); ok {
			p.exitCode = exitError.ExitCode()
		} else {
			p.exitCode = -1
		}
	} else {
		p.exitCode = 0
	}
	p.mu.Unlock()

	// Stop resource monitoring
	if p.resourceMonitor != nil {
		p.resourceMonitor.Stop()
	}

	// Close done channel
	close(p.done)

	p.logger.Info("Process completed",
		"pid", p.pid,
		"exit_code", p.exitCode,
		"duration", endTime.Sub(p.startTime))

	return err
}

// Stop gracefully stops the process
func (p *Process) Stop() error {
	p.mu.RLock()
	if !p.started || p.finished {
		p.mu.RUnlock()
		return nil
	}
	process := p.cmd.Process
	p.mu.RUnlock()

	if process == nil {
		return fmt.Errorf("no process to stop")
	}

	p.logger.Debug("Stopping process gracefully", "pid", p.pid)

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		p.logger.Warn("Failed to send SIGTERM", "pid", p.pid, "error", err)
		return p.Kill()
	}

	// Wait for graceful shutdown with timeout
	select {
	case <-p.done:
		p.logger.Debug("Process stopped gracefully", "pid", p.pid)
		return nil
	case <-time.After(p.killTimeout):
		p.logger.Warn("Process did not stop gracefully, killing", "pid", p.pid)
		return p.Kill()
	}
}

// Kill forcefully kills the process
func (p *Process) Kill() error {
	p.mu.RLock()
	if !p.started {
		p.mu.RUnlock()
		return fmt.Errorf("process not started")
	}
	process := p.cmd.Process
	p.mu.RUnlock()

	if process == nil {
		return fmt.Errorf("no process to kill")
	}

	p.logger.Debug("Killing process", "pid", p.pid)

	// Send SIGKILL
	if err := process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process: %w", err)
	}

	p.logger.Info("Process killed", "pid", p.pid)
	return nil
}

// monitorOutput monitors stdout and stderr and sends to output channel
func (p *Process) monitorOutput() {
	var wg sync.WaitGroup

	// Monitor stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.readOutput(p.stdout, false)
	}()

	// Monitor stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.readOutput(p.stderr, true)
	}()

	wg.Wait()
}

// readOutput reads from a stream and sends to output channel
func (p *Process) readOutput(reader io.ReadCloser, isErr bool) {
	defer reader.Close()

	if p.outputChan == nil {
		return
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text() + "\n"

		select {
		case p.outputChan <- &OutputEvent{
			TaskID:    fmt.Sprintf("%d", p.pid), // Use PID as task ID if not provided
			Line:      line,
			IsErr:     isErr,
			Timestamp: time.Now(),
		}:
		case <-p.ctx.Done():
			return
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		p.logger.Debug("Error reading output", "error", err, "is_err", isErr)
	}
}

// WriteInput writes data to the process stdin
func (p *Process) WriteInput(data []byte) error {
	p.mu.RLock()
	stdin := p.stdin
	p.mu.RUnlock()

	if stdin == nil {
		return fmt.Errorf("stdin not available")
	}

	_, err := stdin.Write(data)
	return err
}

// CloseInput closes the stdin pipe
func (p *Process) CloseInput() error {
	p.mu.RLock()
	stdin := p.stdin
	p.mu.RUnlock()

	if stdin == nil {
		return nil
	}

	return stdin.Close()
}

// GetPID returns the process ID
func (p *Process) GetPID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pid
}

// IsRunning returns true if the process is currently running
func (p *Process) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.started && !p.finished
}

// ExitCode returns the exit code of the process
func (p *Process) ExitCode() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitCode
}

// Error returns any error that occurred
func (p *Process) Error() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.error
}

// StartTime returns when the process started
func (p *Process) StartTime() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.startTime
}

// Duration returns how long the process has been running or ran
func (p *Process) Duration() time.Duration {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.endTime != nil {
		return p.endTime.Sub(p.startTime)
	}

	if p.started {
		return time.Since(p.startTime)
	}

	return 0
}

// GetResourceUsage returns current resource usage
func (p *Process) GetResourceUsage() (*ResourceUsage, error) {
	if p.resourceMonitor == nil {
		return nil, fmt.Errorf("resource monitoring not enabled")
	}

	return p.resourceMonitor.GetCurrentUsage()
}

// GetMaxResourceUsage returns peak resource usage
func (p *Process) GetMaxResourceUsage() (*ResourceUsage, error) {
	if p.resourceMonitor == nil {
		return nil, fmt.Errorf("resource monitoring not enabled")
	}

	return p.resourceMonitor.GetMaxUsage()
}

// Done returns a channel that's closed when the process completes
func (p *Process) Done() <-chan struct{} {
	return p.done
}

// NewResourceMonitor creates a new resource monitor
func NewResourceMonitor(process *Process, logger logger.Logger) *ResourceMonitor {
	return &ResourceMonitor{
		process:  process,
		logger:   logger.WithGroup("resource-monitor"),
		interval: 1 * time.Second, // Monitor every second
		stopChan: make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start starts resource monitoring
func (rm *ResourceMonitor) Start() {
	defer close(rm.done)

	ticker := time.NewTicker(rm.interval)
	defer ticker.Stop()

	rm.logger.Debug("Starting resource monitoring", "pid", rm.process.GetPID())

	for {
		select {
		case <-rm.stopChan:
			rm.logger.Debug("Resource monitoring stopped", "pid", rm.process.GetPID())
			return
		case <-ticker.C:
			if !rm.process.IsRunning() {
				return
			}

			if err := rm.collectMetrics(); err != nil {
				rm.logger.Debug("Failed to collect metrics", "error", err)
			}
		}
	}
}

// Stop stops resource monitoring
func (rm *ResourceMonitor) Stop() {
	select {
	case rm.stopChan <- struct{}{}:
	default:
	}

	// Wait for monitoring to stop
	<-rm.done
}

// collectMetrics collects current resource usage metrics
func (rm *ResourceMonitor) collectMetrics() error {
	pid := rm.process.GetPID()
	if pid == 0 {
		return fmt.Errorf("invalid PID")
	}

	// This is a simplified implementation
	// In a production system, you would use platform-specific APIs
	// or libraries like gopsutil to get accurate resource usage

	usage, err := rm.getProcessStats(pid)
	if err != nil {
		return err
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Update maximums
	if usage.MemoryRSS > rm.maxMemory {
		rm.maxMemory = usage.MemoryRSS
	}

	if usage.CPUPercent > rm.maxCPU {
		rm.maxCPU = usage.CPUPercent
	}

	rm.totalCPUTime = usage.CPUTime

	return nil
}

// getProcessStats gets process statistics (simplified implementation)
func (rm *ResourceMonitor) getProcessStats(pid int) (*ResourceUsage, error) {
	// This is a placeholder implementation
	// In reality, you would read from /proc/<pid>/stat on Linux
	// or use platform-specific APIs on other systems

	return &ResourceUsage{
		PID:         pid,
		MemoryRSS:   0, // Would be populated from system stats
		MemoryVMS:   0, // Would be populated from system stats
		CPUPercent:  0, // Would be calculated from system stats
		CPUTime:     0, // Would be populated from system stats
		NumThreads:  1, // Would be populated from system stats
		NumFDs:      0, // Would be populated from system stats
		StartTime:   rm.process.StartTime(),
		RunningTime: rm.process.Duration(),
	}, nil
}

// GetCurrentUsage returns current resource usage
func (rm *ResourceMonitor) GetCurrentUsage() (*ResourceUsage, error) {
	pid := rm.process.GetPID()
	if pid == 0 {
		return nil, fmt.Errorf("invalid PID")
	}

	return rm.getProcessStats(pid)
}

// GetMaxUsage returns peak resource usage
func (rm *ResourceMonitor) GetMaxUsage() (*ResourceUsage, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return &ResourceUsage{
		PID:         rm.process.GetPID(),
		MemoryRSS:   rm.maxMemory,
		CPUPercent:  rm.maxCPU,
		CPUTime:     rm.totalCPUTime,
		StartTime:   rm.process.StartTime(),
		RunningTime: rm.process.Duration(),
	}, nil
}

// ProcessBuilder helps build process configurations
type ProcessBuilder struct {
	config *ProcessConfig
}

// NewProcessBuilder creates a new process builder
func NewProcessBuilder() *ProcessBuilder {
	return &ProcessBuilder{
		config: &ProcessConfig{
			KillTimeout: 10 * time.Second,
		},
	}
}

// Command sets the command to execute
func (pb *ProcessBuilder) Command(cmd string) *ProcessBuilder {
	pb.config.Command = cmd
	return pb
}

// Args sets the command arguments
func (pb *ProcessBuilder) Args(args ...string) *ProcessBuilder {
	pb.config.Args = args
	return pb
}

// WorkingDir sets the working directory
func (pb *ProcessBuilder) WorkingDir(dir string) *ProcessBuilder {
	pb.config.WorkingDir = dir
	return pb
}

// Env sets environment variables
func (pb *ProcessBuilder) Env(env []string) *ProcessBuilder {
	pb.config.Env = env
	return pb
}

// OutputChan sets the output channel
func (pb *ProcessBuilder) OutputChan(ch chan<- *OutputEvent) *ProcessBuilder {
	pb.config.OutputChan = ch
	return pb
}

// TaskID sets the task ID for output events
func (pb *ProcessBuilder) TaskID(id string) *ProcessBuilder {
	pb.config.TaskID = id
	return pb
}

// KillTimeout sets the timeout for graceful shutdown
func (pb *ProcessBuilder) KillTimeout(timeout time.Duration) *ProcessBuilder {
	pb.config.KillTimeout = timeout
	return pb
}

// Logger sets the logger
func (pb *ProcessBuilder) Logger(logger logger.Logger) *ProcessBuilder {
	pb.config.Logger = logger
	return pb
}

// Build creates the process
func (pb *ProcessBuilder) Build(ctx context.Context) (*Process, error) {
	return NewProcess(ctx, pb.config)
}

// StartAndWait builds, starts, and waits for the process
func (pb *ProcessBuilder) StartAndWait(ctx context.Context) error {
	process, err := pb.Build(ctx)
	if err != nil {
		return err
	}

	if err := process.Start(); err != nil {
		return err
	}

	return process.Wait()
}
