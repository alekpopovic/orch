package policy

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alekpopovic/orch/pkg/types"
)

type ClusterPolicy struct {
	AllowPrivileged         bool     `json:"allow_privileged" yaml:"allow_privileged"`
	AllowHostNetwork        bool     `json:"allow_host_network" yaml:"allow_host_network"`
	AllowHostPID            bool     `json:"allow_host_pid" yaml:"allow_host_pid"`
	AllowedHostPathPrefixes []string `json:"allowed_host_path_prefixes,omitempty" yaml:"allowed_host_path_prefixes"`
	AllowedCapabilities     []string `json:"allowed_capabilities,omitempty" yaml:"allowed_capabilities"`
}

func DefaultClusterPolicy() ClusterPolicy {
	return ClusterPolicy{}
}

func (policy ClusterPolicy) ValidateServiceSpec(spec types.ServiceSpec) error {
	context := spec.SecurityContext
	if context.Privileged && !policy.AllowPrivileged {
		return fmt.Errorf("security_context.privileged is not allowed by cluster policy")
	}
	if context.HostNetwork && !policy.AllowHostNetwork {
		return fmt.Errorf("security_context.host_network is not allowed by cluster policy")
	}
	if context.HostPID && !policy.AllowHostPID {
		return fmt.Errorf("security_context.host_pid is not allowed by cluster policy")
	}
	for _, capability := range context.CapAdd {
		if !policy.capabilityAllowed(capability) {
			return fmt.Errorf("security_context.cap_add %q is not allowed by cluster policy", capability)
		}
	}
	for _, mount := range context.HostPathMounts {
		if !policy.hostPathAllowed(mount.HostPath) {
			return fmt.Errorf("security_context.host_path_mounts host path %q is not allowed by cluster policy", mount.HostPath)
		}
	}
	return nil
}

func (policy ClusterPolicy) capabilityAllowed(capability string) bool {
	capability = normalizeCapability(capability)
	for _, allowed := range policy.AllowedCapabilities {
		if normalizeCapability(allowed) == capability {
			return true
		}
	}
	return false
}

func normalizeCapability(capability string) string {
	return strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(capability)), "CAP_")
}

func (policy ClusterPolicy) hostPathAllowed(path string) bool {
	path = filepath.Clean(path)
	for _, prefix := range policy.AllowedHostPathPrefixes {
		prefix = filepath.Clean(strings.TrimSpace(prefix))
		if prefix == "." || prefix == "" || !strings.HasPrefix(prefix, "/") {
			continue
		}
		if path == prefix || strings.HasPrefix(path, strings.TrimRight(prefix, "/")+"/") {
			return true
		}
	}
	return false
}

func (policy ClusterPolicy) Redacted() map[string]any {
	prefixes := append([]string(nil), policy.AllowedHostPathPrefixes...)
	capabilities := append([]string(nil), policy.AllowedCapabilities...)
	slices.Sort(prefixes)
	slices.Sort(capabilities)
	return map[string]any{
		"allow_privileged":           policy.AllowPrivileged,
		"allow_host_network":         policy.AllowHostNetwork,
		"allow_host_pid":             policy.AllowHostPID,
		"allowed_host_path_prefixes": prefixes,
		"allowed_capabilities":       capabilities,
	}
}
