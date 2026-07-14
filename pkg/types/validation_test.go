package types

import (
	"strings"
	"testing"
	"time"
)

func TestNodeSpecValidate(t *testing.T) {
	valid := NodeSpec{
		Hostname:         "node-1",
		AdvertiseAddress: "10.0.0.10",
		Capacity: Resources{
			CPU:    4000,
			Memory: 8 * 1024 * 1024 * 1024,
		},
		Allocatable: Resources{
			CPU:    3500,
			Memory: 7 * 1024 * 1024 * 1024,
		},
	}

	tests := []struct {
		name    string
		spec    NodeSpec
		wantErr string
	}{
		{
			name: "valid",
			spec: valid,
		},
		{
			name: "hostname required",
			spec: func() NodeSpec {
				spec := valid
				spec.Hostname = ""
				return spec
			}(),
			wantErr: "hostname is required",
		},
		{
			name: "advertise address required",
			spec: func() NodeSpec {
				spec := valid
				spec.AdvertiseAddress = ""
				return spec
			}(),
			wantErr: "advertise address is required",
		},
		{
			name: "capacity cannot be negative",
			spec: func() NodeSpec {
				spec := valid
				spec.Capacity.CPU = -1
				return spec
			}(),
			wantErr: "capacity CPU cannot be negative",
		},
		{
			name: "allocatable cannot exceed capacity",
			spec: func() NodeSpec {
				spec := valid
				spec.Allocatable.Memory = spec.Capacity.Memory + 1
				return spec
			}(),
			wantErr: "allocatable memory cannot exceed capacity memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			assertValidationError(t, err, tt.wantErr)
		})
	}
}

func TestServiceSpecValidate(t *testing.T) {
	valid := ServiceSpec{
		Name:     "web",
		Image:    "nginx:1.27",
		Replicas: 2,
		Env: map[string]string{
			"APP_ENV": "local",
		},
		SecretRefs: []SecretRef{
			{Name: "registry-creds", Key: "password"},
		},
		Ports: []Port{
			{Protocol: PortTCP, ContainerPort: 8080, PublishedPort: 18080},
		},
		ResourceRequirements: ResourceRequirements{
			Requests: Resources{CPU: 100, Memory: 128 * 1024 * 1024},
			Limits:   Resources{CPU: 500, Memory: 512 * 1024 * 1024},
		},
		Healthcheck: &Healthcheck{
			Type:               HealthcheckHTTP,
			Path:               "/healthz",
			Port:               8080,
			Interval:           10 * time.Second,
			Timeout:            2 * time.Second,
			HealthyThreshold:   1,
			UnhealthyThreshold: 3,
		},
		RestartPolicy: RestartPolicy{Condition: RestartOnFailure, MaxAttempts: 3},
		PlacementConstraints: []PlacementConstraint{
			{Key: "region", Operator: ConstraintEquals, Value: "us-east"},
		},
	}

	tests := []struct {
		name    string
		spec    ServiceSpec
		wantErr string
	}{
		{
			name: "valid",
			spec: valid,
		},
		{
			name: "name required",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.Name = ""
				return spec
			}(),
			wantErr: "service name is required",
		},
		{
			name: "image required",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.Image = ""
				return spec
			}(),
			wantErr: "image is required",
		},
		{
			name: "image reference invalid",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.Image = "bad image"
				return spec
			}(),
			wantErr: "image reference is invalid",
		},
		{
			name: "replicas cannot be negative",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.Replicas = -1
				return spec
			}(),
			wantErr: "replicas cannot be negative",
		},
		{
			name: "request cannot exceed limit",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.ResourceRequirements.Requests.CPU = 600
				return spec
			}(),
			wantErr: "requested CPU cannot exceed CPU limit",
		},
		{
			name: "invalid port",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.Ports[0].ContainerPort = 0
				return spec
			}(),
			wantErr: "ports[0]: container port must be between 1 and 65535",
		},
		{
			name: "http healthcheck path required",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.Healthcheck = &Healthcheck{Type: HealthcheckHTTP, Port: 8080}
				return spec
			}(),
			wantErr: "healthcheck path is required for HTTP checks",
		},
		{
			name: "invalid restart policy",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.RestartPolicy.Condition = "sometimes"
				return spec
			}(),
			wantErr: "restart policy condition",
		},
		{
			name: "invalid placement constraint",
			spec: func() ServiceSpec {
				spec := cloneServiceSpec(valid)
				spec.PlacementConstraints[0].Key = ""
				return spec
			}(),
			wantErr: "placement_constraints[0]: key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			assertValidationError(t, err, tt.wantErr)
		})
	}
}

func cloneServiceSpec(spec ServiceSpec) ServiceSpec {
	if spec.Env != nil {
		env := make(map[string]string, len(spec.Env))
		for key, value := range spec.Env {
			env[key] = value
		}
		spec.Env = env
	}
	spec.SecretRefs = append([]SecretRef(nil), spec.SecretRefs...)
	spec.Ports = append([]Port(nil), spec.Ports...)
	spec.PlacementConstraints = append([]PlacementConstraint(nil), spec.PlacementConstraints...)
	if spec.Healthcheck != nil {
		healthcheck := *spec.Healthcheck
		spec.Healthcheck = &healthcheck
	}
	return spec
}

func assertValidationError(t *testing.T, err error, want string) {
	t.Helper()

	if want == "" {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return
	}

	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error containing %q, got %q", want, err.Error())
	}
}
