package api_test

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPISyntax(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	var doc struct {
		OpenAPI    string         `yaml:"openapi"`
		Info       map[string]any `yaml:"info"`
		Paths      map[string]any `yaml:"paths"`
		Components map[string]any `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}
	if doc.OpenAPI == "" || len(doc.Paths) == 0 || len(doc.Components) == 0 {
		t.Fatalf("invalid OpenAPI shape: openapi=%q paths=%d components=%d", doc.OpenAPI, len(doc.Paths), len(doc.Components))
	}
	for _, path := range []string{"/healthz", "/v1/nodes", "/v1/services", "/v1/tasks", "/v1/events", "/v1/logs", "/v1/secrets", "/v1/registry-credentials", "/v1/audit", "/v1/jobs", "/v1/cronjobs", "/v1/volumes", "/v1/volume-claims", "/v1/notification-sinks", "/v1/gitops/status"} {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("OpenAPI spec missing path %s", path)
		}
	}
	for path := range doc.Paths {
		if strings.HasPrefix(path, "/v1/agent/") {
			t.Fatalf("public OpenAPI spec must not expose internal agent path %s", path)
		}
	}
}
