package admission

import (
	"context"
	"strings"
	"testing"

	"github.com/alekpopovic/orch/internal/policy"
	"github.com/alekpopovic/orch/pkg/types"
)

func TestAdmissionPolicies(t *testing.T) {
	base := types.ServiceSpec{
		Name: "api", Image: "registry.example/api:1.0", Replicas: 2,
		ResourceRequirements: types.ResourceRequirements{
			Requests: types.Resources{CPU: 100, Memory: 128},
			Limits:   types.Resources{CPU: 200, Memory: 256},
		},
		Healthcheck: &types.Healthcheck{Type: types.HealthcheckTCP, Port: 8080},
	}
	tests := []struct {
		name   string
		mutate func(*types.ServiceSpec, *policy.ClusterPolicy)
		rule   string
	}{
		{"requests", func(s *types.ServiceSpec, p *policy.ClusterPolicy) {
			p.RequireResourceRequests = true
			s.ResourceRequirements.Requests = types.Resources{}
		}, "resources.requests.required"},
		{"limits", func(s *types.ServiceSpec, p *policy.ClusterPolicy) {
			p.RequireResourceLimits = true
			s.ResourceRequirements.Limits = types.Resources{}
		}, "resources.limits.required"},
		{"privileged", func(s *types.ServiceSpec, _ *policy.ClusterPolicy) { s.SecurityContext.Privileged = true }, "security.privileged.denied"},
		{"host network", func(s *types.ServiceSpec, _ *policy.ClusterPolicy) { s.SecurityContext.HostNetwork = true }, "security.host_network.denied"},
		{"host pid", func(s *types.ServiceSpec, _ *policy.ClusterPolicy) { s.SecurityContext.HostPID = true }, "security.host_pid.denied"},
		{"registry", func(s *types.ServiceSpec, p *policy.ClusterPolicy) {
			p.AllowedImageRegistries = []string{"approved.example"}
		}, "image.registry.denied"},
		{"latest", func(s *types.ServiceSpec, p *policy.ClusterPolicy) {
			p.BlockLatestTag = true
			s.Image = "registry.example/api:latest"
		}, "image.latest.denied"},
		{"healthcheck", func(s *types.ServiceSpec, p *policy.ClusterPolicy) { p.RequireHealthcheck = true; s.Healthcheck = nil }, "healthcheck.required"},
		{"host path", func(s *types.ServiceSpec, p *policy.ClusterPolicy) {
			p.AllowedHostPathPrefixes = []string{"/srv/orch"}
			s.SecurityContext.HostPathMounts = []types.HostPathMount{{HostPath: "/etc", ContainerPath: "/host"}}
		}, "security.host_path.denied"},
		{"replicas", func(s *types.ServiceSpec, p *policy.ClusterPolicy) { p.MaxReplicasPerService = 1 }, "replicas.maximum"},
		{"public ports", func(s *types.ServiceSpec, p *policy.ClusterPolicy) {
			p.MaxPublicPortsPerService = 1
			s.Ports = []types.Port{{PublishedPort: 80}, {PublishedPort: 443}}
		}, "ports.public.maximum"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := base
			clusterPolicy := policy.DefaultClusterPolicy()
			test.mutate(&spec, &clusterPolicy)
			violations := Evaluate(spec, clusterPolicy)
			if len(violations) == 0 || !strings.Contains(violations[0].Rule, test.rule) {
				t.Fatalf("expected rule %q, got %#v", test.rule, violations)
			}
		})
	}
}

type recordingSink struct{ rejection Rejection }

func (sink *recordingSink) RecordAdmissionRejection(_ context.Context, rejection Rejection) {
	sink.rejection = rejection
}

func TestRejectedAdmissionIsAudited(t *testing.T) {
	sink := &recordingSink{}
	controller := New(sink)
	err := controller.Admit(context.Background(), Request{
		Actor: Actor{ID: "alice"}, Operation: OperationCreate, Namespace: "prod",
		Spec:   types.ServiceSpec{Name: "api", Image: "api:latest", SecurityContext: types.SecurityContext{Privileged: true}},
		Policy: policy.DefaultClusterPolicy(),
	})
	if err == nil || sink.rejection.Actor.ID != "alice" || sink.rejection.Namespace != "prod" || len(sink.rejection.Violations) == 0 {
		t.Fatalf("expected audited rejection, err=%v rejection=%#v", err, sink.rejection)
	}
}
