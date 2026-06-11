package types

import (
	"encoding/json"
	"testing"
)

func TestParseCPU(t *testing.T) {
	tests := []struct {
		value string
		want  int64
	}{
		{value: "500m", want: 500},
		{value: "1", want: 1000},
		{value: "2.5", want: 2500},
	}
	for _, tt := range tests {
		got, err := ParseCPU(tt.value)
		if err != nil {
			t.Fatalf("parse CPU %q: %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("parse CPU %q = %d, want %d", tt.value, got, tt.want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		value string
		want  int64
	}{
		{value: "128Mi", want: 128 * 1024 * 1024},
		{value: "512Mi", want: 512 * 1024 * 1024},
		{value: "1Gi", want: 1024 * 1024 * 1024},
		{value: "2G", want: 2 * 1000 * 1000 * 1000},
	}
	for _, tt := range tests {
		got, err := ParseMemory(tt.value)
		if err != nil {
			t.Fatalf("parse memory %q: %v", tt.value, err)
		}
		if got != tt.want {
			t.Fatalf("parse memory %q = %d, want %d", tt.value, got, tt.want)
		}
	}
}

func TestResourcesUnmarshalJSONParsesStrings(t *testing.T) {
	var resources Resources
	if err := json.Unmarshal([]byte(`{"cpu":"2.5","memory":"1Gi"}`), &resources); err != nil {
		t.Fatalf("unmarshal resources: %v", err)
	}
	if resources.CPU != 2500 || resources.Memory != 1024*1024*1024 {
		t.Fatalf("unexpected resources %#v", resources)
	}
}

func TestResourceRequirementsWithDefaults(t *testing.T) {
	requirements, err := (ResourceRequirements{}).WithDefaults(ResourceDefaults{
		Requests: Resources{CPU: 250, Memory: 64 * 1024 * 1024},
		Limits:   Resources{CPU: 500, Memory: 128 * 1024 * 1024},
	})
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if requirements.Requests.CPU != 250 || requirements.Requests.Memory != 64*1024*1024 {
		t.Fatalf("unexpected request defaults %#v", requirements.Requests)
	}
	if requirements.Limits.CPU != 500 || requirements.Limits.Memory != 128*1024*1024 {
		t.Fatalf("unexpected limit defaults %#v", requirements.Limits)
	}
}

func TestResourceRequirementsRejectsInvalidValues(t *testing.T) {
	_, err := (ResourceRequirements{
		Requests: Resources{CPU: 600, Memory: 128 * 1024 * 1024},
		Limits:   Resources{CPU: 500, Memory: 128 * 1024 * 1024},
	}).WithDefaults(DefaultResourceDefaults())
	if err == nil {
		t.Fatalf("expected request exceeding limit to fail")
	}
}
