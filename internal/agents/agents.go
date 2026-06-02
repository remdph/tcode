// Package agents describes the AI coding CLIs tcode can launch (Claude Code and
// its peers) and detects which of them are installed on the host.
package agents

import "os/exec"

// Agent is one launchable coding-agent CLI.
type Agent struct {
	Name string   // display name, e.g. "Claude"
	Bin  string   // executable looked up on PATH, e.g. "claude"
	Args []string // full argv used to start it in interactive (auto-approve) mode
}

// Claude is the default agent and the historical one; used as a fallback when no
// agent at all is detected on PATH.
var Claude = Agent{
	Name: "Claude",
	Bin:  "claude",
	Args: []string{"claude", "--dangerously-skip-permissions"},
}

// Registry is the full set of known agents, in display order. The Args include
// each tool's "auto-approve"/yolo flag where one is known; flags are best-effort
// and easy to tweak here. Only agents whose Bin is on PATH are ever shown.
var Registry = []Agent{
	Claude,
	{Name: "Codex", Bin: "codex", Args: []string{"codex", "--full-auto"}},
	{Name: "Gemini", Bin: "gemini", Args: []string{"gemini", "--yolo"}},
	{Name: "Grok", Bin: "grok", Args: []string{"grok", "--always-approve"}},
	{Name: "OpenCode", Bin: "opencode", Args: []string{"opencode"}},
	{Name: "Aider", Bin: "aider", Args: []string{"aider", "--yes-always"}},
	{Name: "Cursor", Bin: "cursor-agent", Args: []string{"cursor-agent"}},
	{Name: "Amazon Q", Bin: "q", Args: []string{"q", "chat"}},
	{Name: "Qwen", Bin: "qwen", Args: []string{"qwen", "--yolo"}},
	{Name: "Crush", Bin: "crush", Args: []string{"crush"}},
	{Name: "Goose", Bin: "goose", Args: []string{"goose"}},
	{Name: "Copilot", Bin: "copilot", Args: []string{"copilot"}},
}

// Detect returns the registered agents whose executable is found on PATH.
func Detect() []Agent {
	var out []Agent
	for _, a := range Registry {
		if _, err := exec.LookPath(a.Bin); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// Find returns the registered agent with the given Bin.
func Find(bin string) (Agent, bool) {
	for _, a := range Registry {
		if a.Bin == bin {
			return a, true
		}
	}
	return Agent{}, false
}
