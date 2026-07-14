package cli

import (
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestParseDeploy(t *testing.T) {
	spec, err := ParseDeploy([]byte(`
name: api
image: ghcr.io/example/api:1.0.0
imagePullSecret: ghcr-prod
stateful: true
replicas: 3
ports:
  - container: 8080
    public: 80
routes:
  - host: api.example.com
    pathPrefix: /
    port: 8080
    tls: true
env:
  DATABASE_URL:
    secretRef: prod/database-url
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
	if spec.ImagePullSecret != "ghcr-prod" {
		t.Fatalf("unexpected image pull secret %q", spec.ImagePullSecret)
	}
	if !spec.Stateful {
		t.Fatal("expected stateful service")
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
	if spec.Env["NODE_ENV"] != "production" {
		t.Fatalf("unexpected env %#v", spec.Env)
	}
	if len(spec.SecretRefs) != 1 || spec.SecretRefs[0].Name != "prod/database-url" || spec.SecretRefs[0].Env != "DATABASE_URL" {
		t.Fatalf("unexpected secret refs %#v", spec.SecretRefs)
	}
	if len(spec.PlacementConstraints) != 1 || spec.PlacementConstraints[0].Key != "role" {
		t.Fatalf("unexpected constraints %#v", spec.PlacementConstraints)
	}
	if len(spec.Routes) != 1 || spec.Routes[0].Host != "api.example.com" || !spec.Routes[0].TLS {
		t.Fatalf("unexpected routes %#v", spec.Routes)
	}
}

func TestParseDeployValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing required name",
			body: `
image: nginx:1.27
replicas: 1
`,
		},
		{
			name: "invalid image",
			body: `
name: api
image: "bad image"
replicas: 1
`,
		},
		{
			name: "negative replicas",
			body: `
name: api
image: nginx:1.27
replicas: -1
`,
		},
		{
			name: "invalid port",
			body: `
name: api
image: nginx:1.27
replicas: 1
ports:
  - container: 70000
`,
		},
		{
			name: "invalid resources",
			body: `
name: api
image: nginx:1.27
replicas: 1
resources:
  cpu: nope
`,
		},
		{
			name: "invalid healthcheck",
			body: `
name: api
image: nginx:1.27
replicas: 1
healthcheck:
  type: http
  interval: definitely-not-a-duration
`,
		},
		{
			name: "invalid placement labels",
			body: `
name: api
image: nginx:1.27
replicas: 1
placement:
  labels:
    "": worker
`,
		},
		{
			name: "invalid routes",
			body: `
name: api
image: nginx:1.27
replicas: 1
routes:
  - host: api.example.com
    pathPrefix: api
    port: 8080
`,
		},
		{
			name: "invalid secret refs",
			body: `
name: api
image: nginx:1.27
replicas: 1
env:
  DATABASE_URL:
    value: postgres://db
    secretRef: prod/database-url
`,
		},
		{
			name: "unknown field",
			body: `
name: api
image: nginx:1.27
replicas: 1
definitelyUnknown: true
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDeploy([]byte(tt.body)); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}
