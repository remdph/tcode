// Package agents describes the AI coding CLIs tcode can launch (Claude Code and
// its peers) and detects which of them are installed on the host.
package agents

import "os/exec"

// Agent is one launchable coding-agent CLI.
type Agent struct {
	Name string   // display name, e.g. "Claude"
	Bin  string   // executable looked up on PATH, e.g. "claude"
	Args []string // full argv used to start it in interactive (auto-approve) mode
	Env  []string // extra environment ("KEY=VALUE"), for agents that auto-approve via env
}

// Claude is the default agent and the historical one; used as a fallback when no
// agent at all is detected on PATH.
var Claude = Agent{
	Name: "Claude",
	Bin:  "claude",
	Args: []string{"claude", "--dangerously-skip-permissions"},
}

// Registry is the full set of known agents, in display order. Each entry starts
// its CLI in the equivalent of Claude's auto-approve ("yolo") mode, verified
// against each tool's docs (June 2026). Only agents whose Bin is on PATH are
// shown. Flags live here so they are easy to adjust if a tool changes.
var Registry = []Agent{
	Claude,
	// Codex: --full-auto = no prompts for edits/commands inside the workspace
	// (the full bypass is --yolo / --dangerously-bypass-approvals-and-sandbox).
	{Name: "Codex", Bin: "codex", Args: []string{"codex", "--full-auto"}},
	// Gemini: --yolo (alias -y; newer unified form is --approval-mode=yolo).
	{Name: "Gemini", Bin: "gemini", Args: []string{"gemini", "--yolo"}},
	// Grok Build: default is "ask"; --always-approve skips approvals.
	{Name: "Grok", Bin: "grok", Args: []string{"grok", "--always-approve"}},
	// OpenCode: the TUI has no yolo flag; auto-approve is requested via env.
	{Name: "OpenCode", Bin: "opencode", Args: []string{"opencode"},
		Env: []string{"OPENCODE_DANGEROUSLY_SKIP_PERMISSIONS=true"}},
	// Aider: --yes-always auto-confirms every prompt.
	{Name: "Aider", Bin: "aider", Args: []string{"aider", "--yes-always"}},
	// Cursor: --force (alias --yolo) approves trust and enables auto-run.
	{Name: "Cursor", Bin: "cursor-agent", Args: []string{"cursor-agent", "--force"}},
	// Amazon Q: chat session, trusting all tools.
	{Name: "Amazon Q", Bin: "q", Args: []string{"q", "chat", "--trust-all-tools"}},
	// Qwen Code (Gemini-CLI fork): inherits --yolo.
	{Name: "Qwen", Bin: "qwen", Args: []string{"qwen", "--yolo"}},
	// Crush: --yolo bypasses tool-execution confirmation.
	{Name: "Crush", Bin: "crush", Args: []string{"crush", "--yolo"}},
	// Goose: interactive `goose session`; auto-approval via GOOSE_MODE env.
	{Name: "Goose", Bin: "goose", Args: []string{"goose", "session"},
		Env: []string{"GOOSE_MODE=auto"}},
	// GitHub Copilot CLI: --allow-all (alias --yolo) allows tools, paths, URLs.
	{Name: "Copilot", Bin: "copilot", Args: []string{"copilot", "--allow-all"}},
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
