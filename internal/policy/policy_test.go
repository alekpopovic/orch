package policy

import (
	"strings"
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestDefaultPolicyRejectsUnsafeSecurityContext(t *testing.T) {
	tests := []struct {
		name    string
		context types.SecurityContext
		want    string
	}{
		{
			name:    "privileged",
			context: types.SecurityContext{Privileged: true},
			want:    "privileged",
		},
		{
			name:    "host network",
			context: types.SecurityContext{HostNetwork: true},
			want:    "host_network",
		},
		{
			name:    "host pid",
			context: types.SecurityContext{HostPID: true},
			want:    "host_pid",
		},
		{
			name:    "capability add",
			context: types.SecurityContext{CapAdd: []string{"NET_ADMIN"}},
			want:    "cap_add",
		},
		{
			name:    "host path",
			context: types.SecurityContext{HostPathMounts: []types.HostPathMount{{HostPath: "/var/lib/data", ContainerPath: "/data"}}},
			want:    "host_path_mounts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DefaultClusterPolicy().ValidateServiceSpec(serviceSpec(tt.context))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}
}

func TestPolicyAllowsExplicitCapabilitiesAndHostPathPrefixes(t *testing.T) {
	policy := ClusterPolicy{
		AllowedCapabilities:     []string{"CAP_NET_ADMIN"},
		AllowedHostPathPrefixes: []string{"/var/lib/orch-volumes"},
	}
	spec := serviceSpec(types.SecurityContext{
		CapAdd: []string{"NET_ADMIN"},
		HostPathMounts: []types.HostPathMount{{
			HostPath:      "/var/lib/orch-volumes/service-a",
			ContainerPath: "/data",
			ReadOnly:      true,
		}},
	})

	if err := policy.ValidateServiceSpec(spec); err != nil {
		t.Fatalf("expected policy to allow explicit security context: %v", err)
	}
}

func serviceSpec(context types.SecurityContext) types.ServiceSpec {
	return types.ServiceSpec{
		Name:            "web",
		Image:           "nginx:1.27",
		Replicas:        1,
		SecurityContext: context,
	}
}
