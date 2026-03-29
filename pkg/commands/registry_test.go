package commands

import "testing"

func TestRegistry_Definitions_ReturnsCopy(t *testing.T) {
	defs := []Definition{
		{Name: "help", Description: "Show help"},
		{Name: "admin", Description: "Admin command"},
	}
	r := NewRegistry(defs)

	got := r.Definitions()
	if len(got) != 2 {
		t.Fatalf("definitions len = %d, want 2", len(got))
	}

	got[0].Name = "mutated"
	again := r.Definitions()
	if again[0].Name != "help" {
		t.Fatalf("registry should not be mutated by caller, got first name %q", again[0].Name)
	}
}

func TestRegistry_MatchesCommand_ByNameAndAlias(t *testing.T) {
	r := NewRegistry([]Definition{
		{Name: "stop", Aliases: []string{"cancel", "abort"}},
		{Name: "help"},
	})

	cases := []struct {
		text    string
		command string
		want    bool
	}{
		{"/stop", "stop", true},
		{"!stop", "stop", true},
		{"/cancel", "stop", true},
		{"/abort", "stop", true},
		{"/STOP", "stop", true},
		{"/stop", "help", false},
		{"/help", "stop", false},
		{"hello", "stop", false},
	}
	for _, tc := range cases {
		got := r.MatchesCommand(tc.text, tc.command)
		if got != tc.want {
			t.Errorf("MatchesCommand(%q, %q) = %v, want %v", tc.text, tc.command, got, tc.want)
		}
	}
}

func TestRegistry_Lookup_MatchesByLowercaseNameAndAlias(t *testing.T) {
	r := NewRegistry([]Definition{
		{Name: "Help", Aliases: []string{"Assist"}},
		{Name: "List"},
	})

	def, ok := r.Lookup("help")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by lowercase name failed: ok=%v def=%+v", ok, def)
	}

	def, ok = r.Lookup("HELP")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by uppercase name failed: ok=%v def=%+v", ok, def)
	}

	def, ok = r.Lookup("assist")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by lowercase alias failed: ok=%v def=%+v", ok, def)
	}

	def, ok = r.Lookup("ASSIST")
	if !ok || def.Name != "Help" {
		t.Fatalf("lookup by uppercase alias failed: ok=%v def=%+v", ok, def)
	}
}
