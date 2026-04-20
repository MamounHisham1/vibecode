package commands

import (
	"fmt"
	"os"
	"strings"
)

func (r *Registry) registerInit() {
	r.Register(Command{
		Name:        "init",
		Aliases:     []string{"i"},
		Description: "Create or update AGENTS.md for this project",
		Type:        TypePrompt,
		PromptText:  buildInitPrompt(),
	})
}

func buildInitPrompt() string {
	dir, _ := os.Getwd()

	var sb strings.Builder

	sb.WriteString("Create or update AGENTS.md for this repository.\n\n")
	sb.WriteString("The goal is a compact instruction file that helps future AI coding sessions avoid mistakes and ramp up quickly. ")
	sb.WriteString("Every line should answer: \"Would an agent likely miss this without help?\" If not, leave it out.\n\n")

	sb.WriteString("## How to investigate\n\n")
	sb.WriteString("Read the highest-value sources first:\n")
	sb.WriteString("- README*, root manifests, workspace config, lockfiles\n")
	sb.WriteString("- build, test, lint, formatter, typecheck, and codegen config\n")
	sb.WriteString("- CI workflows and pre-commit / task runner config\n")
	sb.WriteString("- existing instruction files (AGENTS.md, CLAUDE.md, VIBECODE.md, .cursor/rules/, .cursorrules, .github/copilot-instructions.md)\n")
	sb.WriteString("- project config files\n\n")
	sb.WriteString("If architecture is still unclear after reading config and docs, inspect a small number of representative code files ")
	sb.WriteString("to find the real entrypoints, package boundaries, and execution flow. ")
	sb.WriteString("Prefer reading the files that explain how the system is wired together over random leaf files.\n\n")
	sb.WriteString("Prefer executable sources of truth over prose. If docs conflict with config or scripts, trust the executable source and only keep what you can verify.\n\n")

	sb.WriteString("## What to extract\n\n")
	sb.WriteString("Look for the highest-signal facts for an agent working in this repo:\n")
	sb.WriteString("- exact developer commands, especially non-obvious ones\n")
	sb.WriteString("- how to run a single test, a single package, or a focused verification step\n")
	sb.WriteString("- required command order when it matters, such as lint -> typecheck -> test\n")
	sb.WriteString("- monorepo or multi-package boundaries, ownership of major directories, and the real app/library entrypoints\n")
	sb.WriteString("- framework or toolchain quirks: generated code, migrations, codegen, build artifacts, special env loading, dev servers, infra deploy flow\n")
	sb.WriteString("- repo-specific style or workflow conventions that differ from defaults\n")
	sb.WriteString("- testing quirks: fixtures, integration test prerequisites, snapshot workflows, required services, flaky or expensive suites\n")
	sb.WriteString("- important constraints from existing instruction files worth preserving\n\n")
	sb.WriteString("Good AGENTS.md content is usually hard-earned context that took reading multiple files to infer.\n\n")

	sb.WriteString("## Questions\n\n")
	sb.WriteString("Only ask the user questions if the repo cannot answer something important. Use the ask_user tool for one short batch at most.\n\n")
	sb.WriteString("Good questions:\n")
	sb.WriteString("- undocumented team conventions\n")
	sb.WriteString("- branch / PR / release expectations\n")
	sb.WriteString("- missing setup or test prerequisites that are known but not written down\n\n")
	sb.WriteString("Do not ask about anything the repo already makes clear.\n\n")

	sb.WriteString("## Writing rules\n\n")
	sb.WriteString("Include only high-signal, repo-specific guidance such as:\n")
	sb.WriteString("- exact commands and shortcuts the agent would otherwise guess wrong\n")
	sb.WriteString("- architecture notes that are not obvious from filenames\n")
	sb.WriteString("- conventions that differ from language or framework defaults\n")
	sb.WriteString("- setup requirements, environment quirks, and operational gotchas\n")
	sb.WriteString("- references to existing instruction sources that matter\n\n")
	sb.WriteString("Exclude:\n")
	sb.WriteString("- generic software advice\n")
	sb.WriteString("- long tutorials or exhaustive file trees\n")
	sb.WriteString("- obvious language conventions\n")
	sb.WriteString("- speculative claims or anything you could not verify\n\n")
	sb.WriteString("When in doubt, omit.\n\n")
	sb.WriteString("Prefer short sections and bullets. If the repo is simple, keep the file simple. ")
	sb.WriteString("If the repo is large, summarize the few structural facts that actually change how an agent should work.\n\n")

	// If AGENTS.md already exists, tell the LLM to improve it in place
	if data, err := os.ReadFile("AGENTS.md"); err == nil {
		sb.WriteString("An AGENTS.md already exists. Its current content is:\n\n")
		sb.WriteString(string(data))
		sb.WriteString("\n\nImprove it in place rather than rewriting blindly. Preserve verified useful guidance, delete fluff or stale claims, and reconcile it with the current codebase.\n\n")
	}

	sb.WriteString(fmt.Sprintf("Write the file to: %s/AGENTS.md", dir))

	return sb.String()
}
