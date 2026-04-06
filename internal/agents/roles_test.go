package agents

import (
	"strings"
	"testing"
)

func TestParseRoles_Single(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	roles, err := ParseRoles("Architect, SOLVER")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
}

func TestParseRoles_Unknown(t *testing.T) {
	t.Parallel()
	_, err := ParseRoles("architect,nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown role, got nil")
	}
	if !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseRoles_UnknownErrorListsTestEngineer(t *testing.T) {
	t.Parallel()
	_, err := ParseRoles("bogus")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "test-engineer") {
		t.Errorf("error should list test-engineer in available roles: %v", err)
	}
}

func TestParseRoles_ErrorListsAllRoles(t *testing.T) {
	t.Parallel()
	_, err := ParseRoles("bogus")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for slug := range roleMap {
		if !strings.Contains(err.Error(), slug) {
			t.Errorf("error should list %q in available roles: %v", slug, err)
		}
	}
}

func TestParseRoles_AllRoles(t *testing.T) {
	t.Parallel()
	input := "know-it-all,architect,solver,editor,optimizer,sentinel,test-engineer"
	roles, err := ParseRoles(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 7 {
		t.Errorf("expected 7 roles, got %d", len(roles))
	}
}

func TestAllRoles_Count(t *testing.T) {
	t.Parallel()
	if len(AllRoles) != 7 {
		t.Errorf("expected 7 roles in AllRoles, got %d", len(AllRoles))
	}
}

func TestAllRoles_HaveSkillFiles(t *testing.T) {
	t.Parallel()
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

func TestAllRoles_HaveSpecialties(t *testing.T) {
	t.Parallel()
	for _, r := range AllRoles {
		if len(r.Specialties) == 0 {
			t.Errorf("role %q has no specialties", r.Slug)
		}
	}
}

func TestIsDomainAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role     string
		category string
		want     bool
	}{
		{"sentinel", CategorySecurity, true},
		{"sentinel", CategoryStyle, false},
		{"architect", CategoryDesign, true},
		{"architect", CategoryBug, false},
		{"know-it-all", CategoryStyle, true},
		{"know-it-all", CategoryDesign, true},
		{"know-it-all", CategorySecurity, false},
		{"solver", CategoryBug, true},
		{"editor", CategoryStyle, true},
		{"optimizer", CategoryPerformance, true},
		{"test-engineer", CategoryTesting, true},
		{"nonexistent", CategoryBug, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.role+"/"+tt.category, func(t *testing.T) {
			t.Parallel()
			got := IsDomainAuthority(tt.role, tt.category)
			if got != tt.want {
				t.Errorf("IsDomainAuthority(%q, %q) = %v, want %v", tt.role, tt.category, got, tt.want)
			}
		})
	}
}
