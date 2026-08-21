package version

import "testing"

func TestAgentCompatibility(t *testing.T) {
	cases := map[string]Compatibility{"0.1.9": TooOld, "0.2.0": Compatible, "0.3.0": Compatible, "0.4.0": UntestedNewer}
	for input, want := range cases {
		got, err := CheckAgent(input)
		if err != nil || got != want {
			t.Fatalf("%s got=%s err=%v", input, got, err)
		}
	}
}
func TestSchemaCompatibility(t *testing.T) {
	if CheckSchema(MinimumSchema) != nil || CheckSchema(MaximumSchema) != nil {
		t.Fatal("supported schema rejected")
	}
	if CheckSchema(MinimumSchema-1) == nil {
		t.Fatal("old schema accepted")
	}
	if CheckSchema(MaximumSchema+1) == nil {
		t.Fatal("future schema accepted")
	}
}
