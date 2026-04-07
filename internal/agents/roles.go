package agents

import (
	"fmt"
	"strings"
)

// Role represents a reviewer persona.
type Role struct {
	Name        string
	Slug        string
	Description string
	SkillFile   string       // path to the Claude skill file
	Model       string       // preferred model for this role (empty = use global default)
	Specialties []string     // finding categories this role is a domain authority on
	Relevance   []PRCategory // PR categories this role is relevant for; nil = all
}

// Model tier constants for per-role defaults.
// These are aliases that the Claude CLI resolves to specific model versions.
const (
	ModelTierDeep     = "opus"   // deep reasoning: architecture, security
	ModelTierStandard = "sonnet" // good reasoning: best practices, correctness, performance
	ModelTierFast     = "haiku"  // pattern matching: style, testing
)

var (
	RoleKnowItAll = Role{
		Name:        "Know-It-All",
		Slug:        "know-it-all",
		Description: "Best practices, code smells, and language idioms",
		SkillFile:   "skills/know-it-all.md",
		Model:       ModelTierStandard,
		Specialties: []string{CategoryStyle, CategoryDesign},
		// Relevance: nil — relevant for all PR categories.
	}
	RoleArchitect = Role{
		Name:        "Architect",
		Slug:        "architect",
		Description: "Code patterns, system fit, scalability, and abstractions",
		SkillFile:   "skills/architect.md",
		Model:       ModelTierDeep,
		Specialties: []string{CategoryDesign},
		Relevance:   []PRCategory{PRCategoryCode},
	}
	RoleSolver = Role{
		Name:        "Solver",
		Slug:        "solver",
		Description: "Problem coverage and solution completeness",
		SkillFile:   "skills/solver.md",
		Model:       ModelTierStandard,
		Specialties: []string{CategoryBug},
		Relevance:   []PRCategory{PRCategoryCode, PRCategoryTests},
	}
	RoleEditor = Role{
		Name:        "Editor",
		Slug:        "editor",
		Description: "Readability, brevity, simplicity, and duplication",
		SkillFile:   "skills/editor.md",
		Model:       ModelTierFast,
		Specialties: []string{CategoryStyle},
		Relevance:   []PRCategory{PRCategoryCode, PRCategoryDocs},
	}
	RoleOptimizer = Role{
		Name:        "Optimizer",
		Slug:        "optimizer",
		Description: "Performance, complexity, and optimization",
		SkillFile:   "skills/optimizer.md",
		Model:       ModelTierStandard,
		Specialties: []string{CategoryPerformance},
		Relevance:   []PRCategory{PRCategoryCode},
	}
	RoleSentinel = Role{
		Name:        "Sentinel",
		Slug:        "sentinel",
		Description: "Security vulnerabilities, dangerous code, and attack vectors",
		SkillFile:   "skills/sentinel.md",
		Model:       ModelTierDeep,
		Specialties: []string{CategorySecurity},
		Relevance:   []PRCategory{PRCategoryCode, PRCategoryConfig},
	}
	RoleTestEngineer = Role{
		Name:        "Test Engineer",
		Slug:        "test-engineer",
		Description: "Test coverage, edge cases, and testing improvements",
		SkillFile:   "skills/test-engineer.md",
		Model:       ModelTierFast,
		Specialties: []string{CategoryTesting},
		Relevance:   []PRCategory{PRCategoryCode, PRCategoryTests},
	}

	AllRoles = []Role{
		RoleKnowItAll,
		RoleArchitect,
		RoleSolver,
		RoleEditor,
		RoleOptimizer,
		RoleSentinel,
		RoleTestEngineer,
	}

	roleMap = map[string]Role{
		"know-it-all":   RoleKnowItAll,
		"architect":     RoleArchitect,
		"solver":        RoleSolver,
		"editor":        RoleEditor,
		"optimizer":     RoleOptimizer,
		"sentinel":      RoleSentinel,
		"test-engineer": RoleTestEngineer,
	}
)

// IsDomainAuthority returns true if the role is a domain expert for the given
// finding category. Domain authority votes carry extra weight in composite
// severity calculations.
func IsDomainAuthority(roleSlug, findingCategory string) bool {
	role, ok := roleMap[roleSlug]
	if !ok {
		return false
	}
	for _, s := range role.Specialties {
		if s == findingCategory {
			return true
		}
	}
	return false
}

// ParseRoles parses a comma-separated list of role slugs.
func ParseRoles(input string) ([]Role, error) {
	parts := strings.Split(input, ",")
	roles := make([]Role, 0, len(parts))

	for _, p := range parts {
		slug := strings.TrimSpace(strings.ToLower(p))
		role, ok := roleMap[slug]
		if !ok {
			available := make([]string, 0, len(roleMap))
			for k := range roleMap {
				available = append(available, k)
			}
			return nil, fmt.Errorf("unknown role: %q (available: %s)", slug, strings.Join(available, ", "))
		}
		roles = append(roles, role)
	}

	if len(roles) == 0 {
		return nil, fmt.Errorf("no valid roles specified")
	}

	return roles, nil
}
