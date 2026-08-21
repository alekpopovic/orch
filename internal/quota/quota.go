package quota

import (
	"fmt"
	"strings"

	"github.com/alekpopovic/orch/pkg/types"
)

type Error struct {
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
	Limit     int64  `json:"limit"`
	Used      int64  `json:"used"`
	Requested int64  `json:"requested"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("namespace %q quota exceeded for %s: limit %d, used %d, requested %d", e.Namespace, e.Resource, e.Limit, e.Used, e.Requested)
}

func Validate(value types.ResourceQuota) error {
	if strings.TrimSpace(value.Namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if value.MaxServices < 0 || value.MaxReplicas < 0 || value.MaxCPUMillicores < 0 || value.MaxMemoryBytes < 0 || value.MaxPublicPorts < 0 || value.MaxSecrets < 0 || value.MaxRegistryCredentials < 0 {
		return fmt.Errorf("quota limits cannot be negative")
	}
	return nil
}

func Check(value types.ResourceQuota, usage types.ResourceUsage) error {
	checks := []struct {
		name  string
		limit int64
		used  int64
	}{
		{"services", int64(value.MaxServices), int64(usage.Services)},
		{"replicas", int64(value.MaxReplicas), int64(usage.Replicas)},
		{"cpu_millicores", value.MaxCPUMillicores, usage.CPUMillicores},
		{"memory_bytes", value.MaxMemoryBytes, usage.MemoryBytes},
		{"public_ports", int64(value.MaxPublicPorts), int64(usage.PublicPorts)},
		{"secrets", int64(value.MaxSecrets), int64(usage.Secrets)},
		{"registry_credentials", int64(value.MaxRegistryCredentials), int64(usage.RegistryCredentials)},
	}
	for _, check := range checks {
		if check.limit > 0 && check.used > check.limit {
			return &Error{Namespace: value.Namespace, Resource: check.name, Limit: check.limit, Used: check.used, Requested: check.used}
		}
	}
	return nil
}
