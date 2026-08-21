package admission

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alekpopovic/orch/internal/policy"
	"github.com/alekpopovic/orch/pkg/types"
)

type Operation string

const (
	OperationCreate  Operation = "create"
	OperationUpdate  Operation = "update"
	OperationRollout Operation = "rollout"
	OperationScale   Operation = "scale"
)

type Actor struct {
	ID   string `json:"id"`
	Role string `json:"role,omitempty"`
}

type Request struct {
	Actor     Actor
	Operation Operation
	Namespace string
	Spec      types.ServiceSpec
	Policy    policy.ClusterPolicy
}

type Violation struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type Error struct {
	Operation  Operation   `json:"operation"`
	Namespace  string      `json:"namespace"`
	Violations []Violation `json:"violations"`
}

func (e *Error) Error() string {
	if e == nil || len(e.Violations) == 0 {
		return "admission rejected"
	}
	return fmt.Sprintf("admission rejected: %s", e.Violations[0].Message)
}

type Rejection struct {
	Actor      Actor
	Operation  Operation
	Namespace  string
	Service    string
	Violations []Violation
}

type AuditSink interface {
	RecordAdmissionRejection(context.Context, Rejection)
}

type Controller struct {
	sink AuditSink
}

func New(sink AuditSink) *Controller {
	return &Controller{sink: sink}
}

func (controller *Controller) Admit(ctx context.Context, request Request) error {
	violations := Evaluate(request.Spec, request.Policy)
	if len(violations) == 0 {
		return nil
	}
	if controller != nil && controller.sink != nil {
		controller.sink.RecordAdmissionRejection(ctx, Rejection{
			Actor: request.Actor, Operation: request.Operation, Namespace: request.Namespace,
			Service: request.Spec.Name, Violations: violations,
		})
	}
	return &Error{Operation: request.Operation, Namespace: request.Namespace, Violations: violations}
}

func Evaluate(spec types.ServiceSpec, clusterPolicy policy.ClusterPolicy) []Violation {
	violations := make([]Violation, 0)
	add := func(field, rule, message string) {
		violations = append(violations, Violation{Field: field, Rule: rule, Message: message})
	}

	if clusterPolicy.RequireResourceRequests && (spec.ResourceRequirements.Requests.CPU <= 0 || spec.ResourceRequirements.Requests.Memory <= 0) {
		add("resource_requirements.requests", "resources.requests.required", "CPU and memory resource requests are required")
	}
	if clusterPolicy.RequireResourceLimits && (spec.ResourceRequirements.Limits.CPU <= 0 || spec.ResourceRequirements.Limits.Memory <= 0) {
		add("resource_requirements.limits", "resources.limits.required", "CPU and memory resource limits are required")
	}
	if spec.SecurityContext.Privileged && !clusterPolicy.AllowPrivileged {
		add("security_context.privileged", "security.privileged.denied", "privileged containers are not allowed")
	}
	if spec.SecurityContext.HostNetwork && !clusterPolicy.AllowHostNetwork {
		add("security_context.host_network", "security.host_network.denied", "host networking is not allowed")
	}
	if spec.SecurityContext.HostPID && !clusterPolicy.AllowHostPID {
		add("security_context.host_pid", "security.host_pid.denied", "host PID namespace is not allowed")
	}
	for i, capability := range spec.SecurityContext.CapAdd {
		if !containsFold(clusterPolicy.AllowedCapabilities, strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(capability)), "CAP_")) {
			add(fmt.Sprintf("security_context.cap_add[%d]", i), "security.capability.denied", fmt.Sprintf("Linux capability %q is not allowed", capability))
		}
	}
	for i, mount := range spec.SecurityContext.HostPathMounts {
		if !pathAllowed(mount.HostPath, clusterPolicy.AllowedHostPathPrefixes) {
			add(fmt.Sprintf("security_context.host_path_mounts[%d].host_path", i), "security.host_path.denied", fmt.Sprintf("host path %q is not allowed", mount.HostPath))
		}
	}
	if len(clusterPolicy.AllowedImageRegistries) > 0 {
		registry := imageRegistry(spec.Image)
		if !containsFold(clusterPolicy.AllowedImageRegistries, registry) {
			add("image", "image.registry.denied", fmt.Sprintf("image registry %q is not allowed", registry))
		}
	}
	if clusterPolicy.BlockLatestTag && usesLatestTag(spec.Image) {
		add("image", "image.latest.denied", "the latest image tag is not allowed")
	}
	if clusterPolicy.RequireHealthcheck && spec.Healthcheck == nil {
		add("healthcheck", "healthcheck.required", "a healthcheck is required")
	}
	if clusterPolicy.MaxReplicasPerService > 0 && spec.Replicas > clusterPolicy.MaxReplicasPerService {
		add("replicas", "replicas.maximum", fmt.Sprintf("replicas cannot exceed %d", clusterPolicy.MaxReplicasPerService))
	}
	if clusterPolicy.MaxPublicPortsPerService > 0 {
		public := 0
		for _, port := range spec.Ports {
			if port.PublishedPort > 0 {
				public++
			}
		}
		if public > clusterPolicy.MaxPublicPortsPerService {
			add("ports", "ports.public.maximum", fmt.Sprintf("public ports cannot exceed %d", clusterPolicy.MaxPublicPortsPerService))
		}
	}
	return violations
}

func containsFold(values []string, target string) bool {
	target = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(target)), "cap_")
	for _, value := range values {
		value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "cap_")
		if value == target {
			return true
		}
	}
	return false
}

func pathAllowed(path string, prefixes []string) bool {
	path = filepath.Clean(path)
	for _, prefix := range prefixes {
		prefix = filepath.Clean(strings.TrimSpace(prefix))
		if strings.HasPrefix(prefix, "/") && (path == prefix || strings.HasPrefix(path, strings.TrimRight(prefix, "/")+"/")) {
			return true
		}
	}
	return false
}

func imageRegistry(image string) string {
	first, _, hasSlash := strings.Cut(strings.TrimSpace(image), "/")
	if !hasSlash || (!strings.Contains(first, ".") && !strings.Contains(first, ":") && first != "localhost") {
		return "docker.io"
	}
	return strings.ToLower(first)
}

func usesLatestTag(image string) bool {
	image = strings.TrimSpace(image)
	if at := strings.IndexByte(image, '@'); at >= 0 {
		return false
	}
	lastSlash := strings.LastIndexByte(image, '/')
	lastColon := strings.LastIndexByte(image, ':')
	return lastColon <= lastSlash || strings.EqualFold(image[lastColon+1:], "latest")
}
