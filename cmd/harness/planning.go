package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	PlanningAuto       = "auto"
	PlanningSpecDriven = "spec-driven"
	PlanningContract   = "contract"
	PlanningManual     = "manual"
)

func normalizePlanningMode(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "detect":
		return PlanningAuto
	case "spec", "spec-driven", "spec_driven", "specdriven", "full", "tlc":
		return PlanningSpecDriven
	case "contract", "contracts", "contract-only", "contract_only", "skills", "on":
		return PlanningContract
	case "manual", "off", "none", "false", "no", "n", "nao", "0":
		return PlanningManual
	}
	return v
}

func planningModeFromSkills(skills string) (string, error) {
	switch normalizeSkillsMode(skills) {
	case "auto":
		return PlanningAuto, nil
	case "on":
		return PlanningSpecDriven, nil
	case "off":
		return PlanningManual, nil
	default:
		return "", fmt.Errorf("unknown skills mode %q; use auto|on|off", skills)
	}
}

func planningUsesSkills(mode string) bool {
	return mode == PlanningSpecDriven || mode == PlanningContract
}

func planningModeLabel(mode string) string {
	switch mode {
	case PlanningSpecDriven:
		return "spec-driven automation"
	case PlanningContract:
		return "contract automation only"
	case PlanningManual:
		return "manual contracts"
	default:
		return mode
	}
}

func boolSkillsModeForPlanning(mode string) string {
	if planningUsesSkills(mode) {
		return "on"
	}
	return "off"
}

func specDrivenSkillsInstalled(root string) bool {
	_, err := os.Stat(filepath.Join(root, "skills", "spec-driven", "SKILL.md"))
	return err == nil
}

func planningModeFromInstalled(root string) string {
	if specDrivenSkillsInstalled(root) {
		return PlanningSpecDriven
	}
	if skillsInstalled(root) {
		return PlanningContract
	}
	return PlanningManual
}
