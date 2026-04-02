package agents

import (
	"strings"
	"testing"
)

func TestParseRoles_Single(t *testing.T) {
	roles, err := ParseRoles("architect")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roles))
	}
	if roles[0].Slug != "architect" {
		t.Errorf("expected slug 'architect', got %q", roles[0].Slug)
	}
}

func TestParseRoles_Multiple(t *testing.T) {
	roles, err := ParseRoles("know-it-all, editor, sentinel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(roles))
	}
	slugs := make([]string, len(roles))
	for i, r := range roles {
		slugs[i] = r.Slug
	}
	expected := []string{"know-it-all", "editor", "sentinel"}
	for i, want := range expected {
		if slugs[i] != want {
			t.Errorf("role %d: expected %q, got %q", i, want, slugs[i])
		}
	}
}

func TestParseRoles_CaseInsensitive(t *testing.T) {
	roles, err := ParseRoles("Architect, SOLVER")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
}

func TestParseRoles_Unknown(t *testing.T) {
	_, err := ParseRoles("architect,nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown role, got nil")
	}
	if !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseRoles_AllRoles(t *testing.T) {
	input := "know-it-all,architect,solver,editor,optimizer,sentinel"
	roles, err := ParseRoles(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 6 {
		t.Errorf("expected 6 roles, got %d", len(roles))
	}
}

func TestAllRoles_Count(t *testing.T) {
	if len(AllRoles) != 7 {
		t.Errorf("expected 7 roles in AllRoles, got %d", len(AllRoles))
	}
}

func TestAllRoles_HaveSkillFiles(t *testing.T) {
	for _, r := range AllRoles {
		if r.SkillFile == "" {
			t.Errorf("role %q has empty SkillFile", r.Slug)
		}
		if r.Name == "" {
			t.Errorf("role %q has empty Name", r.Slug)
		}
		if r.Description == "" {
			t.Errorf("role %q has empty Description", r.Slug)
		}
	}
}
