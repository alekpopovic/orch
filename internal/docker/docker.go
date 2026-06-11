package docker

import "context"

type Runtime interface {
	EnsureContainer(ctx context.Context, spec ContainerSpec) error
	RemoveContainer(ctx context.Context, name string) error
}

type ContainerSpec struct {
	Name  string
	Image string
	Ports []PortBinding
	Env   map[string]string
}

type PortBinding struct {
	ContainerPort int
	HostPort      int
	Protocol      string
}
