package tern

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// defaultAgentCommand is used when Tern's settings don't name an agent command
const defaultAgentCommand = "claude"

// Available reports whether the tern CLI is on PATH
func Available() bool {
	_, err := exec.LookPath("tern")
	return err == nil
}

// run executes a tern command and returns stdout
func run(args ...string) ([]byte, error) {
	cmd := exec.Command("tern", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tern %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// configDir returns Tern's settings directory
func configDir() (string, error) {
	if d := os.Getenv("TERN_CONFIG_DIR"); d != "" {
		return d, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "tern"), nil
}

// AgentCommand returns the command Tern's agent blocks run (settings.json agent_command)
func AgentCommand() string {
	dir, err := configDir()
	if err != nil {
		return defaultAgentCommand
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return defaultAgentCommand
	}

	var settings struct {
		AgentCommand string `json:"agent_command"`
	}
	if err := json.Unmarshal(data, &settings); err != nil || settings.AgentCommand == "" {
		return defaultAgentCommand
	}
	return settings.AgentCommand
}

// loginShell returns the user's shell
func loginShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

// SessionExists reports whether a session with this name exists
func SessionExists(name string) (bool, error) {
	out, err := run("ls", "--json")
	if err != nil {
		return false, err
	}

	var result struct {
		Sessions []struct {
			Name string `json:"name"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return false, fmt.Errorf("parsing tern ls: %w", err)
	}

	for _, s := range result.Sessions {
		if s.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// NewWorkspaceSession creates a session named name in dir with an agent block
// running Tern's agent command, and a terminal block beside it
func NewWorkspaceSession(name, dir string) error {
	// The agent command may be a shell function or alias, so run it through a login shell
	out, err := run("new", "session", name, "--cwd", dir, "--json", "--",
		loginShell(), "-l", "-c", AgentCommand())
	if err != nil {
		return err
	}

	var created struct {
		Block uint64 `json:"block"`
	}
	if err := json.Unmarshal(out, &created); err != nil {
		return fmt.Errorf("parsing tern new session: %w", err)
	}

	_, err = run("split", fmt.Sprint(created.Block), "right", "--cwd", dir)
	return err
}

// KillSession ends a session and every program running in it
func KillSession(name string) error {
	_, err := run("kill", "session", name)
	return err
}
