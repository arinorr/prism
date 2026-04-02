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
	SkillFile   string // path to the Claude skill file
}

var (
	RoleKnowItAll = Role{
		Name:        "Know-It-All",
		Slug:        "know-it-all",
		Description: "Best practices, code smells, and language idioms",
		SkillFile:   "skills/know-it-all.md",
	}
	RoleArchitect = Role{
		Name:        "Architect",
		Slug:        "architect",
		Description: "Code patterns, system fit, scalability, and abstractions",
		SkillFile:   "skills/architect.md",
	}
	RoleSolver = Role{
		Name:        "Solver",
		Slug:        "solver",
		Description: "Problem coverage and solution completeness",
		SkillFile:   "skills/solver.md",
	}
	RoleEditor = Role{
		Name:        "Editor",
		Slug:        "editor",
		Description: "Readability, brevity, simplicity, and duplication",
		SkillFile:   "skills/editor.md",
	}
	RoleOptimizer = Role{
		Name:        "Optimizer",
		Slug:        "optimizer",
		Description: "Performance, complexity, and optimization",
		SkillFile:   "skills/optimizer.md",
	}
	RoleSentinel = Role{
		Name:        "Sentinel",
		Slug:        "sentinel",
		Description: "Security vulnerabilities, dangerous code, and attack vectors",
		SkillFile:   "skills/sentinel.md",
	}
	RoleTestEngineer = Role{
		Name:        "Test Engineer",
		Slug:        "test-engineer",
		Description: "Test coverage, edge cases, and testing improvements",
		SkillFile:   "skills/test-engineer.md",
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

// ParseRoles parses a comma-separated list of role slugs.
func ParseRoles(input string) ([]Role, error) {
	parts := strings.Split(input, ",")
	roles := make([]Role, 0, len(parts))

	for _, p := range parts {
		slug := strings.TrimSpace(strings.ToLower(p))
		role, ok := roleMap[slug]
		if !ok {
			return nil, fmt.Errorf("unknown role: %q (available: know-it-all, architect, solver, editor, optimizer, sentinel)", slug)
		}
		roles = append(roles, role)
	}

	if len(roles) == 0 {
		return nil, fmt.Errorf("no valid roles specified")
	}

	return roles, nil
}
