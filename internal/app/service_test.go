package app

import "testing"

func TestParseToolsDefault(t *testing.T) {
	tools, err := ParseTools("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
}

func TestParseToolsInvalid(t *testing.T) {
	_, err := ParseTools("codex,unknown")
	if err == nil {
		t.Fatalf("expected error")
	}
}
