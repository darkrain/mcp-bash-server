package server

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
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

func NewMCPServer(cfg *config.Config, logger *slog.Logger, version string) (*mcp.Server, *sysinfo.SystemInfo, *ProcessRegistry) {
	si := sysinfo.GetSystemInfo()

	processTTL := time.Duration(cfg.Bash.ProcessTTL) * time.Minute
	if processTTL <= 0 {
		processTTL = 60 * time.Minute
	}
	registry := NewProcessRegistry(processTTL)

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

		if cfg.Bash.LogCommands {
			logger.Info("command started", "command", redactCommand(cmdStr), "cwd", input.Cwd, "timeout", timeout)
		}

		var stdoutBuf, stderrBuf bytes.Buffer

		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()
		}

		cmd := exec.CommandContext(ctx, input.Command, args...)
		if input.Cwd != "" {
			cmd.Dir = input.Cwd
		}
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf
		if timeout > 0 {
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		}

		var err error
		if startErr := cmd.Start(); startErr == nil {
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err = <-done:
			case <-ctx.Done():
				if cmd.Process != nil {
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				}
				<-done
				err = ctx.Err()
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

		elapsed := time.Since(p.StartedAt).Milliseconds()
		result := map[string]any{
			"process_id": p.ID,
			"status":     string(p.Status),
			"command":    p.Command,
			"elapsed_ms": elapsed,
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
			text = fmt.Sprintf("Process %s is still running (%d ms elapsed)", p.ID, elapsed)
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
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Process %s is still running. Use process_status to check when it completes.", p.ID)},
				},
			}, nil, nil
		}

		var content []mcp.Content
		if p.Stdout != "" {
			content = append(content, &mcp.TextContent{
				Text: fmt.Sprintf("=== STDOUT ===\n%s", p.Stdout),
			})
		}
		if p.Stderr != "" {
			content = append(content, &mcp.TextContent{
				Text: fmt.Sprintf("=== STDERR ===\n%s", p.Stderr),
			})
		}
		if len(content) == 0 {
			content = append(content, &mcp.TextContent{
				Text: fmt.Sprintf("Process completed with exit code %d (no output)", p.ExitCode),
			})
		}

		return &mcp.CallToolResult{
				Content: content,
			}, map[string]any{
				"process_id": p.ID,
				"exit_code":  p.ExitCode,
				"stdout":     p.Stdout,
				"stderr":     p.Stderr,
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

		registry.Update(input.ProcessID, func(p *Process) {
			if p.cancel != nil {
				p.cancel()
			}
		})

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Kill signal sent to process %s", input.ProcessID)},
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
			elapsed := time.Since(p.StartedAt).Milliseconds()
			line := fmt.Sprintf("%s | %s | %s", p.ID, p.Status, p.Command)
			if p.Status == StatusRunning {
				line += fmt.Sprintf(" (%dms elapsed)", elapsed)
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

	return mcpServer, si, registry
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
	ctx, cancel := context.WithCancel(context.Background())

	registry.Update(p.ID, func(proc *Process) {
		proc.cancel = cancel
	})

	var stdoutBuf, stderrBuf bytes.Buffer

	args := []string{"-c", p.Command}
	cmdStr := "/bin/bash" + " " + strings.Join(args, " ")

	if cfg.Bash.LogCommands {
		logger.Info("async command executing", "process_id", p.ID, "command", redactCommand(cmdStr))
	}

	cmd := exec.CommandContext(ctx, "/bin/bash", args...)
	if p.Cwd != "" {
		cmd.Dir = p.Cwd
	}
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var err error
	if startErr := cmd.Start(); startErr == nil {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case err = <-done:
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			<-done
			err = ctx.Err()
		}
	} else {
		err = startErr
	}

	now := time.Now()
	exitCode := 0
	status := StatusCompleted

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if status2, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode = status2.ExitStatus()
			}
		} else if ctx.Err() != nil {
			exitCode = -1
			status = StatusKilled
		} else {
			exitCode = -1
			status = StatusFailed
		}
	}

	stdout, stderr := truncateOutputs(stdoutBuf.String(), stderrBuf.String(), cfg.Bash.MaxOutputSize)
	if !utf8.ValidString(stdout) {
		stdout = "[binary output truncated]"
	}
	if !utf8.ValidString(stderr) {
		stderr = "[binary output truncated]"
	}

	registry.Update(p.ID, func(proc *Process) {
		proc.Status = status
		proc.ExitCode = exitCode
		proc.Stdout = stdout
		proc.Stderr = stderr
		proc.EndedAt = &now
		proc.Duration = now.Sub(proc.StartedAt).Milliseconds()
		proc.cancel = nil
	})

	if cfg.Bash.LogCommands {
		logger.Info("async command completed", "process_id", p.ID, "status", string(status), "exit_code", exitCode, "duration_ms", now.Sub(p.StartedAt).Milliseconds())
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
