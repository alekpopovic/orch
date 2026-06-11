package cli

import (
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestParseDeploy(t *testing.T) {
	spec, err := ParseDeploy([]byte(`
name: api
image: ghcr.io/example/api:1.0.0
replicas: 3
ports:
  - container: 8080
    public: 80
env:
  NODE_ENV: production
resources:
  cpu: 500m
  memory: 512Mi
healthcheck:
  type: http
  path: /health
  interval: 10s
  timeout: 2s
  healthyThreshold: 2
  unhealthyThreshold: 4
restart:
  policy: always
placement:
  labels:
    role: app
`))
	if err != nil {
		t.Fatalf("parse deploy: %v", err)
	}

	if spec.Name != "api" {
		t.Fatalf("expected name api, got %q", spec.Name)
	}
	if spec.Image != "ghcr.io/example/api:1.0.0" {
		t.Fatalf("unexpected image %q", spec.Image)
	}
	if spec.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %d", spec.Replicas)
	}
	if spec.ResourceRequirements.Requests.CPU != 500 {
		t.Fatalf("expected 500 millicpu, got %d", spec.ResourceRequirements.Requests.CPU)
	}
	if spec.ResourceRequirements.Requests.Memory != 512*1024*1024 {
		t.Fatalf("unexpected memory %d", spec.ResourceRequirements.Requests.Memory)
	}
	if spec.Healthcheck == nil || spec.Healthcheck.Port != 8080 || spec.Healthcheck.Path != "/health" {
		t.Fatalf("unexpected healthcheck %#v", spec.Healthcheck)
	}
	if spec.Healthcheck.HealthyThreshold != 2 || spec.Healthcheck.UnhealthyThreshold != 4 {
		t.Fatalf("unexpected healthcheck thresholds %#v", spec.Healthcheck)
	}
	if spec.RestartPolicy.Condition != types.RestartAlways {
		t.Fatalf("unexpected restart policy %q", spec.RestartPolicy.Condition)
	}
	if len(spec.PlacementConstraints) != 1 || spec.PlacementConstraints[0].Key != "role" {
		t.Fatalf("unexpected constraints %#v", spec.PlacementConstraints)
	}
}

func TestParseDeployValidation(t *testing.T) {
	_, err := ParseDeploy([]byte(`
name: api
replicas: -1
resources:
  cpu: nope
`))
	if err == nil {
		t.Fatalf("expected validation error")
	}
}
