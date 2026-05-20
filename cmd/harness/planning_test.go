package harness

import "testing"

func TestPlanningModeFromLegacySkillsOnDefaultsToSpecDriven(t *testing.T) {
	mode, err := planningModeFromSkills("on")
	if err != nil {
		t.Fatal(err)
	}
	if mode != PlanningSpecDriven {
		t.Fatalf("expected skills=on to map to spec-driven, got %q", mode)
	}
}

func TestSetupPlanningExplicitModeWinsOverLegacySkills(t *testing.T) {
	mode, err := setupPlanning(setupOptions{
		Planning: PlanningContract,
		Skills:   "off",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if mode != PlanningContract {
		t.Fatalf("expected explicit planning mode, got %q", mode)
	}
}

func TestSetupPlanningNonInteractiveAutoDefaultsToSpecDriven(t *testing.T) {
	mode, err := setupPlanning(setupOptions{
		Planning: PlanningAuto,
		Skills:   "auto",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if mode != PlanningSpecDriven {
		t.Fatalf("expected non-interactive setup to default to spec-driven, got %q", mode)
	}
}

func TestResolveHookPlanningLegacyOffMapsManual(t *testing.T) {
	mode, err := resolveHookPlanning(PlanningAuto, "off")
	if err != nil {
		t.Fatal(err)
	}
	if mode != PlanningManual {
		t.Fatalf("expected skills=off to map to manual, got %q", mode)
	}
}
