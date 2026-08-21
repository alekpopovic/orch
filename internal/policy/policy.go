package policy

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alekpopovic/orch/pkg/types"
)

type ClusterPolicy struct {
	RequireResourceRequests  bool     `json:"require_resource_requests" yaml:"require_resource_requests"`
	RequireResourceLimits    bool     `json:"require_resource_limits" yaml:"require_resource_limits"`
	AllowPrivileged          bool     `json:"allow_privileged" yaml:"allow_privileged"`
	AllowHostNetwork         bool     `json:"allow_host_network" yaml:"allow_host_network"`
	AllowHostPID             bool     `json:"allow_host_pid" yaml:"allow_host_pid"`
	AllowedImageRegistries   []string `json:"allowed_image_registries,omitempty" yaml:"allowed_image_registries"`
	BlockLatestTag           bool     `json:"block_latest_tag" yaml:"block_latest_tag"`
	RequireHealthcheck       bool     `json:"require_healthcheck" yaml:"require_healthcheck"`
	AllowedHostPathPrefixes  []string `json:"allowed_host_path_prefixes,omitempty" yaml:"allowed_host_path_prefixes"`
	AllowedCapabilities      []string `json:"allowed_capabilities,omitempty" yaml:"allowed_capabilities"`
	MaxReplicasPerService    int      `json:"max_replicas_per_service,omitempty" yaml:"max_replicas_per_service"`
	MaxPublicPortsPerService int      `json:"max_public_ports_per_service,omitempty" yaml:"max_public_ports_per_service"`
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
	registries := append([]string(nil), policy.AllowedImageRegistries...)
	slices.Sort(prefixes)
	slices.Sort(capabilities)
	slices.Sort(registries)
	return map[string]any{
		"require_resource_requests":    policy.RequireResourceRequests,
		"require_resource_limits":      policy.RequireResourceLimits,
		"allow_privileged":             policy.AllowPrivileged,
		"allow_host_network":           policy.AllowHostNetwork,
		"allow_host_pid":               policy.AllowHostPID,
		"allowed_image_registries":     registries,
		"block_latest_tag":             policy.BlockLatestTag,
		"require_healthcheck":          policy.RequireHealthcheck,
		"allowed_host_path_prefixes":   prefixes,
		"allowed_capabilities":         capabilities,
		"max_replicas_per_service":     policy.MaxReplicasPerService,
		"max_public_ports_per_service": policy.MaxPublicPortsPerService,
	}
}
