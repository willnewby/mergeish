package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// clis lists supported AI CLIs in order of preference. Each is invoked
// as `<name> -p <prompt>` and expected to print a response to stdout.
var clis = []string{"claude", "pi"}

// FindCLI returns the first available AI CLI on PATH
func FindCLI() (string, error) {
	for _, name := range clis {
		if _, err := exec.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("no AI CLI found on PATH (looked for: %s)", strings.Join(clis, ", "))
}

// PRContent holds AI-generated pull request content
type PRContent struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

const prPrompt = `You are generating a pull request title and body for a change that spans one or more git repositories managed as a single workspace.

Based on the branch name, commit messages, and diffs below, respond with ONLY a JSON object in this exact format, with no other text before or after it:
{"title": "<concise PR title, max 72 chars, imperative mood>", "body": "<PR body in markdown: a short summary paragraph, then a '## Changes' section with bullet points>"}

`

// GeneratePR runs the AI CLI to generate a PR title and body from the given context
func GeneratePR(cli, context string) (*PRContent, error) {
	cmd := exec.Command(cli, "-p", prPrompt+context)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s -p: %w: %s", cli, err, stderr.String())
	}

	content, err := parsePRContent(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("parsing %s output: %w", cli, err)
	}

	return content, nil
}

// parsePRContent extracts the JSON object from the CLI output, tolerating
// surrounding prose or markdown code fences
func parsePRContent(output string) (*PRContent, error) {
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("no JSON object found in output: %s", truncate(output, 200))
	}

	var content PRContent
	if err := json.Unmarshal([]byte(output[start:end+1]), &content); err != nil {
		return nil, err
	}

	if content.Title == "" {
		return nil, fmt.Errorf("AI response missing title")
	}

	return &content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
