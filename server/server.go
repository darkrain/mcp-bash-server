package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"mcp-bash-server/config"
	"mcp-bash-server/sysinfo"
)

type BashInput struct {
	Command string   `json:"command" jsonschema:"the bash command to execute"`
	Args    []string `json:"args,omitempty" jsonschema:"optional arguments for the command"`
	Timeout int      `json:"timeout,omitempty" jsonschema:"optional timeout in seconds, overrides default"`
	Cwd     string   `json:"cwd,omitempty" jsonschema:"optional working directory"`
}

type BashTimeoutResult struct {
	ProcessID string `json:"process_id"`
	Message   string `json:"message"`
}

type BashAsyncInput struct {
	Command string `json:"command" jsonschema:"the bash command to execute asynchronously"`
	Cwd     string `json:"cwd,omitempty" jsonschema:"optional working directory"`
}

type ProcessStatusInput struct {
	ProcessID string `json:"process_id" jsonschema:"the ID returned by bash_async"`
}

type ProcessKillInput struct {
	ProcessID string `json:"process_id" jsonschema:"the ID of the process to kill"`
}

func boolPtr(b bool) *bool {
	return &b
}

func maybeRedactToken(tok string) string {
	lower := strings.ToLower(tok)
	for _, keyword := range []string{"password", "secret", "token", "key"} {
		if strings.Contains(lower, keyword) {
			for _, sep := range []string{"=", ":"} {
				if idx := strings.Index(tok, sep); idx >= 0 {
					return tok[:idx+1] + "***REDACTED***"
				}
			}
		}
	}
	return tok
}

func redactCommand(cmd string) string {
	tokens := strings.Split(cmd, " ")
	for i, tok := range tokens {
		tokens[i] = maybeRedactToken(tok)
	}
	return strings.Join(tokens, " ")
}

type BashOutput struct {
	Stdout   string `json:"stdout" jsonschema:"standard output of the command"`
	Stderr   string `json:"stderr" jsonschema:"standard error of the command"`
	ExitCode int    `json:"exit_code" jsonschema:"exit code of the command"`
	Duration int64  `json:"duration_ms" jsonschema:"execution duration in milliseconds"`
}

func NewMCPServer(cfg *config.Config, logger *slog.Logger, version string) (*mcp.Server, *sysinfo.SystemInfo, *ProcessRegistry, error) {
	si := sysinfo.GetSystemInfo()

	processTTL := time.Duration(cfg.Bash.ProcessTTL) * time.Minute
	if processTTL <= 0 {
		processTTL = 60 * time.Minute
	}
	registry, err := NewProcessRegistry(cfg.Bash.ProcessDir, processTTL, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to initialize process registry: %w", err)
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-bash-server",
		Version: version,
	}, nil)

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "bash",
		Description: si.ToolDescription(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Bash Command Executor",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BashInput) (*mcp.CallToolResult, BashOutput, error) {
		start := time.Now()

		if strings.TrimSpace(input.Command) == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Error: command is empty"},
				},
			}, BashOutput{}, nil
		}

		if !isCommandAllowed(input.Command, cfg) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error: command '%s' is not in the allowed commands list", strings.Fields(input.Command)[0])},
				},
			}, BashOutput{}, nil
		}

		timeout := cfg.Bash.Timeout
		if input.Timeout > 0 {
			timeout = input.Timeout
		}

		var args []string
		if len(input.Args) > 0 {
			args = input.Args
		} else {
			args = []string{"-c", input.Command}
			input.Command = "/bin/bash"
		}

		cmdStr := input.Command
		if len(args) > 0 {
			cmdStr = input.Command + " " + strings.Join(args, " ")
		}

		if wouldKillServer(cmdStr, cfg) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error: command would kill the MCP server process (port %d, PID %d). This is blocked to prevent self-destruction.", cfg.Server.Port, os.Getpid())},
				},
			}, BashOutput{}, nil
		}

		if cfg.Bash.LogCommands {
			logger.Info("command started", "command", redactCommand(cmdStr), "cwd", input.Cwd, "timeout", timeout)
		}

		var stdoutBuf, stderrBuf bytes.Buffer

		timeoutCh := make(chan struct{})
		if timeout > 0 {
			go func() {
				time.Sleep(time.Duration(timeout) * time.Second)
				close(timeoutCh)
			}()
		}

		cmd := exec.Command(input.Command, args...)
		if input.Cwd != "" {
			cmd.Dir = input.Cwd
		}
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		if timeout > 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		}

		var err error
		var asyncProcess *Process
		if startErr := cmd.Start(); startErr == nil {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err = <-done:
			case <-timeoutCh:
				asyncProcess = registry.NewProcess(cmdStr, input.Cwd)

				outFile, fileErr := registry.CreateOutputFile(asyncProcess.ID)
				if fileErr == nil {
					outFile.Write(stdoutBuf.Bytes())
					outFile.Write(stderrBuf.Bytes())
					outFile.Close()
				}

				registry.Update(asyncProcess.ID, func(proc *Process) {
					proc.PID = cmd.Process.Pid
				})

				go func() {
					waitErr := <-done
					now := time.Now()
					exitCode := 0
					status := StatusCompleted
					if waitErr != nil {
						if exitErr, ok := waitErr.(*exec.ExitError); ok {
							if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
								if ws.Signaled() {
									exitCode = -1
									status = StatusKilled
								} else {
									exitCode = ws.ExitStatus()
									status = StatusFailed
								}
							} else {
								exitCode = exitErr.ExitCode()
								status = StatusFailed
							}
						} else {
							exitCode = -1
							status = StatusFailed
						}
					}
					registry.Update(asyncProcess.ID, func(proc *Process) {
						proc.Status = status
						proc.ExitCode = exitCode
						proc.EndedAt = &now
						proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
					})
					if cfg.Bash.LogCommands {
						logger.Info("async timeout process completed", "process_id", asyncProcess.ID, "pid", cmd.Process.Pid, "status", string(status), "exit_code", exitCode)
					}
				}()

				if cfg.Bash.LogCommands {
					logger.Info("command timed out, transferred to async", "command", redactCommand(cmdStr), "process_id", asyncProcess.ID, "timeout", timeout)
				}

				stdout, stderr := truncateOutputs(stdoutBuf.String(), stderrBuf.String(), cfg.Bash.MaxOutputSize)
				if !utf8.ValidString(stdout) {
					stdout = "[binary output truncated]"
				}
				if !utf8.ValidString(stderr) {
					stderr = "[binary output truncated]"
				}

				msg := fmt.Sprintf("Command is still running after %d seconds (timeout reached). It has been moved to background execution.\nProcess ID: %s\n\nThe command has NOT failed — it is still executing in the background.\nUse process_status to check progress, process_output to get results when it finishes, process_kill to terminate it.", timeout, asyncProcess.ID)
				if stdout != "" {
					msg += fmt.Sprintf("\n\n=== STDOUT (so far) ===\n%s", stdout)
				}
				if stderr != "" {
					msg += fmt.Sprintf("\n\n=== STDERR (so far) ===\n%s", stderr)
				}

				return &mcp.CallToolResult{
						Content: []mcp.Content{
							&mcp.TextContent{Text: msg},
						},
					}, BashOutput{
						Stdout:   stdout,
						Stderr:   stderr,
						ExitCode: -1,
						Duration: time.Since(start).Milliseconds(),
					}, nil
			}
		} else {
			err = startErr
		}
		duration := time.Since(start).Milliseconds()

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				}
			} else {
				exitCode = -1
			}
		}

		if cfg.Bash.LogCommands {
			logger.Info("command completed", "command", redactCommand(cmdStr), "exit_code", exitCode, "duration_ms", duration)
		}

		stdout, stderr := truncateOutputs(stdoutBuf.String(), stderrBuf.String(), cfg.Bash.MaxOutputSize)

		if !utf8.ValidString(stdout) {
			stdout = "[binary output truncated]"
		}
		if !utf8.ValidString(stderr) {
			stderr = "[binary output truncated]"
		}

		output := BashOutput{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
			Duration: duration,
		}

		if err != nil && exitCode == -1 {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Execution error: %v\n\nStdout:\n%s\n\nStderr:\n%s", err, stdout, stderr)},
				},
			}, output, nil
		}

		var content []mcp.Content

		if stdout != "" {
			content = append(content, &mcp.TextContent{
				Text: fmt.Sprintf("=== STDOUT ===\n%s", stdout),
			})
		}
		if stderr != "" {
			content = append(content, &mcp.TextContent{
				Text: fmt.Sprintf("=== STDERR ===\n%s", stderr),
			})
		}

		if len(content) == 0 {
			content = append(content, &mcp.TextContent{
				Text: fmt.Sprintf("Command completed with exit code %d in %d ms", exitCode, duration),
			})
		}

		return &mcp.CallToolResult{
			Content: content,
		}, output, nil
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "bash_async",
		Description: si.AsyncToolDescription(),
		Annotations: &mcp.ToolAnnotations{
			Title:           "Async Bash Command Executor",
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			IdempotentHint:  false,
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input BashAsyncInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(input.Command) == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Error: command is empty"},
				},
			}, nil, nil
		}

		if !isCommandAllowed(input.Command, cfg) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error: command '%s' is not in the allowed commands list", strings.Fields(input.Command)[0])},
				},
			}, nil, nil
		}

		if wouldKillServer(input.Command, cfg) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error: command would kill the MCP server process (port %d, PID %d). This is blocked to prevent self-destruction.", cfg.Server.Port, os.Getpid())},
				},
			}, nil, nil
		}

		p := registry.NewProcess(input.Command, input.Cwd)

		if cfg.Bash.LogCommands {
			logger.Info("async command started", "process_id", p.ID, "command", redactCommand(input.Command), "cwd", input.Cwd)
		}

		go runAsyncProcess(p, registry, cfg, logger)

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Process started. ID: %s\nStatus: running\n\nUse process_status to check progress, process_output to get results, process_kill to terminate.", p.ID)},
			},
		}, map[string]any{"process_id": p.ID, "status": "running"}, nil
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "process_status",
		Description: "Check the status of an asynchronously launched process. Returns current status (running/completed/failed/killed), elapsed time, and exit code if finished.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Process Status Check",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProcessStatusInput) (*mcp.CallToolResult, any, error) {
		p, ok := registry.Get(input.ProcessID)
		if !ok {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error: process '%s' not found. It may have expired or never existed. Use process_list to see active processes.", input.ProcessID)},
				},
			}, nil, nil
		}

		if p.Status == StatusRunning && !isPIDAlive(p.PID) {
			now := time.Now()
			registry.Update(p.ID, func(proc *Process) {
				proc.Status = StatusFailed
				proc.ExitCode = -1
				proc.EndedAt = &now
				proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
			})
			p, _ = registry.Get(p.ID)
		}

		elapsed := time.Since(p.StartedAt).Milliseconds()
		result := map[string]any{
			"process_id": p.ID,
			"status":     string(p.Status),
			"command":    p.Command,
			"elapsed_ms": elapsed,
		}
		if p.PID > 0 {
			result["pid"] = p.PID
		}
		if p.ExitCode != 0 || p.Status != StatusRunning {
			result["exit_code"] = p.ExitCode
		}
		if p.Duration > 0 {
			result["duration_ms"] = p.Duration
		}

		var text string
		switch p.Status {
		case StatusRunning:
			text = fmt.Sprintf("Process %s is still running (PID %d, %d ms elapsed)", p.ID, p.PID, elapsed)
		case StatusCompleted:
			text = fmt.Sprintf("Process %s completed with exit code %d in %d ms", p.ID, p.ExitCode, p.Duration)
		case StatusFailed:
			text = fmt.Sprintf("Process %s failed with exit code %d in %d ms", p.ID, p.ExitCode, p.Duration)
		case StatusKilled:
			text = fmt.Sprintf("Process %s was killed after %d ms", p.ID, p.Duration)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, result, nil
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "process_output",
		Description: "Get the output (stdout and stderr) of a completed async process. The process must be in completed/failed/killed state. Use process_status first to check if it's done.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Process Output Reader",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProcessStatusInput) (*mcp.CallToolResult, any, error) {
		p, ok := registry.Get(input.ProcessID)
		if !ok {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error: process '%s' not found", input.ProcessID)},
				},
			}, nil, nil
		}

		if p.Status == StatusRunning {
			if !isPIDAlive(p.PID) {
				now := time.Now()
				registry.Update(p.ID, func(proc *Process) {
					proc.Status = StatusFailed
					proc.ExitCode = -1
					proc.EndedAt = &now
					proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
				})
				p, _ = registry.Get(p.ID)
			} else {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{
						&mcp.TextContent{Text: fmt.Sprintf("Process %s is still running. Use process_status to check when it completes.", p.ID)},
					},
				}, nil, nil
			}
		}

		output, err := registry.ReadOutput(p, cfg.Bash.MaxOutputSize)
		if err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error reading output: %v", err)},
				},
			}, nil, nil
		}

		var content []mcp.Content
		if output != "" {
			content = append(content, &mcp.TextContent{
				Text: output,
			})
		} else {
			content = append(content, &mcp.TextContent{
				Text: fmt.Sprintf("Process completed with exit code %d (no output)", p.ExitCode),
			})
		}

		return &mcp.CallToolResult{
				Content: content,
			}, map[string]any{
				"process_id": p.ID,
				"exit_code":  p.ExitCode,
			}, nil
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "process_kill",
		Description: "Kill a running async process by its ID. Only works for processes that are still running.",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Process Killer",
			DestructiveHint: boolPtr(true),
			OpenWorldHint:   boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ProcessKillInput) (*mcp.CallToolResult, any, error) {
		p, ok := registry.Get(input.ProcessID)
		if !ok {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error: process '%s' not found", input.ProcessID)},
				},
			}, nil, nil
		}

		if p.Status != StatusRunning {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Process %s is not running (status: %s)", p.ID, p.Status)},
				},
			}, nil, nil
		}

		if isPIDAlive(p.PID) {
			_ = syscall.Kill(-p.PID, syscall.SIGKILL)
		}

		now := time.Now()
		registry.Update(input.ProcessID, func(proc *Process) {
			proc.Status = StatusKilled
			proc.ExitCode = -1
			proc.EndedAt = &now
			proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
		})

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Kill signal sent to process %s (PID %d)", input.ProcessID, p.PID)},
			},
		}, nil, nil
	})

	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "process_list",
		Description: "List all async processes and their current statuses. Useful to find running or recently completed processes.",
		Annotations: &mcp.ToolAnnotations{
			Title:         "Process List",
			ReadOnlyHint:  true,
			OpenWorldHint: boolPtr(false),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		processes := registry.List()
		if len(processes) == 0 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "No async processes found."},
				},
			}, nil, nil
		}

		var lines []string
		for _, p := range processes {
			line := fmt.Sprintf("%s | %s | %s", p.ID, p.Status, p.Command)
			if p.Status == StatusRunning {
				elapsed := time.Since(p.StartedAt).Milliseconds()
				line += fmt.Sprintf(" (PID %d, %dms elapsed)", p.PID, elapsed)
			} else if p.Duration > 0 {
				line += fmt.Sprintf(" (exit %d, %dms)", p.ExitCode, p.Duration)
			}
			lines = append(lines, line)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: strings.Join(lines, "\n")},
			},
		}, nil, nil
	})

	return mcpServer, si, registry, nil
}

func isCommandAllowed(command string, cfg *config.Config) bool {
	if len(cfg.Bash.AllowedCommands) == 0 {
		return true
	}
	for _, allowed := range cfg.Bash.AllowedCommands {
		if allowed == "*" || allowed == "all" {
			return true
		}
	}
	cmdName := strings.Fields(command)[0]
	for _, allowed := range cfg.Bash.AllowedCommands {
		if cmdName == allowed {
			return true
		}
	}
	return false
}

func runAsyncProcess(p *Process, registry *ProcessRegistry, cfg *config.Config, logger *slog.Logger) {
	outFile, err := registry.CreateOutputFile(p.ID)
	if err != nil {
		now := time.Now()
		registry.Update(p.ID, func(proc *Process) {
			proc.Status = StatusFailed
			proc.ExitCode = -1
			proc.EndedAt = &now
			proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
		})
		if cfg.Bash.LogCommands {
			logger.Error("async command failed to create output file", "process_id", p.ID, "error", err)
		}
		return
	}
	defer outFile.Close()

	args := []string{"-c", p.Command}
	cmdStr := "/bin/bash" + " " + strings.Join(args, " ")

	if cfg.Bash.LogCommands {
		logger.Info("async command executing", "process_id", p.ID, "command", redactCommand(cmdStr))
	}

	cmd := exec.Command("/bin/bash", args...)
	if p.Cwd != "" {
		cmd.Dir = p.Cwd
	}
	cmd.Stdout = outFile
	cmd.Stderr = outFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err = cmd.Start()
	if err != nil {
		now := time.Now()
		registry.Update(p.ID, func(proc *Process) {
			proc.Status = StatusFailed
			proc.ExitCode = -1
			proc.EndedAt = &now
			proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
		})
		if cfg.Bash.LogCommands {
			logger.Error("async command failed to start", "process_id", p.ID, "error", err)
		}
		return
	}

	registry.Update(p.ID, func(proc *Process) {
		proc.PID = cmd.Process.Pid
	})

	waitErr := cmd.Wait()

	now := time.Now()
	exitCode := 0
	status := StatusCompleted

	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if ws.Signaled() {
					exitCode = -1
					status = StatusKilled
				} else {
					exitCode = ws.ExitStatus()
					status = StatusFailed
				}
			}
		} else {
			exitCode = -1
			status = StatusFailed
		}
	}

	registry.Update(p.ID, func(proc *Process) {
		proc.Status = status
		proc.ExitCode = exitCode
		proc.EndedAt = &now
		proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
	})

	if cfg.Bash.LogCommands {
		logger.Info("async command completed", "process_id", p.ID, "pid", cmd.Process.Pid, "status", string(status), "exit_code", exitCode, "duration_ms", now.Sub(p.StartedAt).Milliseconds())
	}
}

func truncateOutputs(stdout, stderr string, maxTotal int) (string, string) {
	if maxTotal <= 0 {
		return stdout, stderr
	}
	total := len(stdout) + len(stderr)
	if total <= maxTotal {
		return stdout, stderr
	}
	if stdout == "" {
		return "", truncateString(stderr, maxTotal)
	}
	if stderr == "" {
		return truncateString(stdout, maxTotal), ""
	}
	ratio := float64(len(stdout)) / float64(total)
	maxOut := int(float64(maxTotal) * ratio)
	maxErr := maxTotal - maxOut
	if maxOut < 256 {
		maxOut = 256
		maxErr = maxTotal - maxOut
	}
	if maxErr < 256 {
		maxErr = 256
		maxOut = maxTotal - maxErr
	}
	return truncateString(stdout, maxOut), truncateString(stderr, maxErr)
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n... [output truncated]"
}

func wouldKillServer(cmd string, cfg *config.Config) bool {
	lower := strings.ToLower(cmd)
	portStr := strconv.Itoa(cfg.Server.Port)
	pidStr := strconv.Itoa(os.Getpid())

	killKeywords := []string{"kill", "fuser -k", "fuser -kill", "pkill", "killall", "systemctl stop", "systemctl kill", "systemctl restart", "service stop"}
	hasKillIntent := false
	for _, kw := range killKeywords {
		if strings.Contains(lower, kw) {
			hasKillIntent = true
			break
		}
	}
	if !hasKillIntent {
		return false
	}

	if strings.Contains(lower, "mcp-bash-server") {
		return true
	}

	if strings.Contains(cmd, pidStr) {
		return true
	}

	if strings.Contains(cmd, portStr) {
		return true
	}

	return false
}
