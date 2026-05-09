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

func NewMCPServer(cfg *config.Config, logger *slog.Logger) (*mcp.Server, *sysinfo.SystemInfo) {
	si := sysinfo.GetSystemInfo()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-bash-server",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
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

		if len(cfg.Bash.AllowedCommands) > 0 {
			// Check for wildcard "*" or "all" to allow any command
			allowsAll := false
			for _, allowed := range cfg.Bash.AllowedCommands {
				if allowed == "*" || allowed == "all" {
					allowsAll = true
					break
				}
			}

			if !allowsAll {
				cmdName := strings.Fields(input.Command)[0]
				found := false
				for _, allowed := range cfg.Bash.AllowedCommands {
					if cmdName == allowed {
						found = true
						break
					}
				}
				if !found {
					return &mcp.CallToolResult{
						IsError: true,
						Content: []mcp.Content{
							&mcp.TextContent{Text: fmt.Sprintf("Error: command '%s' is not in the allowed commands list", cmdName)},
						},
					}, BashOutput{}, nil
				}
			}
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

	return server, si
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
