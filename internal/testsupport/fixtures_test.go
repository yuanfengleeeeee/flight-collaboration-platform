package testsupport

import "testing"

func TestCandidateScenarioContainsRequiredCases(t *testing.T) {
	scenario, err := NewCandidateScenario("b2-case")
	if err != nil {
		t.Fatal(err)
	}
	if len(scenario.Users) != 4 || len(scenario.TeamMembers) != 4 || len(scenario.PersonnelStatuses) != 4 {
		t.Fatalf("unexpected fixture sizes: users=%d members=%d statuses=%d", len(scenario.Users), len(scenario.TeamMembers), len(scenario.PersonnelStatuses))
	}
	if scenario.TaskTemplate.TriggerEventType != "flight_arrived" {
		t.Fatalf("unexpected trigger type: %s", scenario.TaskTemplate.TriggerEventType)
	}
	if scenario.Users[0].Password == FixturePassword || scenario.Users[0].Password == "" {
		t.Fatal("fixture password must be stored as a hash")
	}
}

func TestCandidateScenarioUsesDistinctPrefix(t *testing.T) {
	first, err := NewCandidateScenario("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCandidateScenario("second")
	if err != nil {
		t.Fatal(err)
	}
	if first.Team.Code == second.Team.Code || first.Flight.FlightNo == second.Flight.FlightNo {
		t.Fatal("fixture prefixes must isolate scenarios")
	}
}
