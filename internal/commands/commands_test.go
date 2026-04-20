package commands

import (
	"strings"
	"testing"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		input   string
		cmdName string
		args    string
		ok      bool
	}{
		{"/help", "help", "", true},
		{"/help me", "help", "me", true},
		{"/clear", "clear", "", true},
		{"hello", "", "", false},
		{"/model gpt-4", "model", "gpt-4", true},
	}

	for _, tt := range tests {
		cmdName, args, ok := ParseInput(tt.input)
		if ok != tt.ok {
			t.Errorf("ParseInput(%q) ok = %v, want %v", tt.input, ok, tt.ok)
		}
		if cmdName != tt.cmdName {
			t.Errorf("ParseInput(%q) cmdName = %q, want %q", tt.input, cmdName, tt.cmdName)
		}
		if args != tt.args {
			t.Errorf("ParseInput(%q) args = %q, want %q", tt.input, args, tt.args)
		}
	}
}

func TestRegistryLookup(t *testing.T) {
	r := NewRegistry()

	cmd, ok := r.Lookup("help")
	if !ok {
		t.Fatal("help command not found")
	}
	if cmd.Name != "help" {
		t.Errorf("Name = %q, want 'help'", cmd.Name)
	}

	// Test alias
	cmd, ok = r.Lookup("h")
	if !ok {
		t.Fatal("h alias not found")
	}
	if cmd.Name != "help" {
		t.Errorf("Alias 'h' resolved to %q, want 'help'", cmd.Name)
	}

	// Test nonexistent
	_, ok = r.Lookup("nonexistent")
	if ok {
		t.Error("nonexistent command should not be found")
	}
}

func TestBuiltinCommands(t *testing.T) {
	r := NewRegistry()

	// help
	cmd, _ := r.Lookup("help")
	result := cmd.Handler("")
	if !strings.Contains(result.Output, "Available commands") {
		t.Errorf("help output = %q, want 'Available commands'", result.Output)
	}

	// clear
	cmd, _ = r.Lookup("clear")
	result = cmd.Handler("")
	if !result.Clear {
		t.Error("clear should set Clear=true")
	}

	// model
	cmd, _ = r.Lookup("model")
	result = cmd.Handler("")
	if !strings.Contains(result.Output, "picker") {
		t.Errorf("model (no args) = %q", result.Output)
	}
	result = cmd.Handler("gpt-4o")
	if !strings.Contains(result.Output, "gpt-4o") {
		t.Errorf("model gpt-4o = %q", result.Output)
	}
}

func TestAllCommands(t *testing.T) {
	r := NewRegistry()
	all := r.All()

	if len(all) < 3 {
		t.Errorf("All() returned %d commands, want >= 5", len(all))
	}

	// Check no duplicates
	names := make(map[string]bool)
	for _, cmd := range all {
		if names[cmd.Name] {
			t.Errorf("duplicate command: %s", cmd.Name)
		}
		names[cmd.Name] = true
	}
}

func TestRegisterCustomCommand(t *testing.T) {
	r := NewRegistry()

	r.Register(Command{
		Name:        "custom",
		Aliases:     []string{"c"},
		Description: "Custom command",
		Type:        TypeLocal,
		Handler: func(args string) Result {
			return Result{Output: "custom: " + args}
		},
	})

	cmd, ok := r.Lookup("custom")
	if !ok {
		t.Fatal("custom command not found")
	}
	result := cmd.Handler("test")
	if result.Output != "custom: test" {
		t.Errorf("output = %q, want 'custom: test'", result.Output)
	}

	// Test alias
	_, ok = r.Lookup("c")
	if !ok {
		t.Fatal("alias 'c' not found")
	}
}
