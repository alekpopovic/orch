package types

import (
	"fmt"
	"strconv"
	"strings"
)

func ParsePortSpec(value string) (Port, error) {
	protocol := PortTCP
	main := strings.TrimSpace(value)
	if before, after, ok := strings.Cut(main, "/"); ok {
		main = before
		protocol = PortProtocol(strings.ToLower(after))
	}
	parts := strings.Split(main, ":")
	if len(parts) < 1 || len(parts) > 2 {
		return Port{}, fmt.Errorf("port must be [published:]container[/protocol]")
	}
	containerIndex := len(parts) - 1
	container, err := strconv.Atoi(parts[containerIndex])
	if err != nil {
		return Port{}, fmt.Errorf("container port: %w", err)
	}
	published := 0
	if len(parts) == 2 {
		published, err = strconv.Atoi(parts[0])
		if err != nil {
			return Port{}, fmt.Errorf("published port: %w", err)
		}
	}
	port := Port{Protocol: protocol, ContainerPort: container, PublishedPort: published}
	return port, port.Validate()
}
func ParseLabelSelector(value string) ([]PlacementConstraint, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	out := []PlacementConstraint{}
	for _, part := range strings.Split(value, ",") {
		operator := ConstraintEquals
		key, val, ok := strings.Cut(part, "!=")
		if ok {
			operator = ConstraintNotEquals
		} else {
			key, val, ok = strings.Cut(part, "=")
		}
		if !ok {
			return nil, fmt.Errorf("selector %q requires = or !=", part)
		}
		constraint := PlacementConstraint{Key: strings.TrimSpace(key), Operator: operator, Value: strings.TrimSpace(val)}
		if err := constraint.Validate(); err != nil {
			return nil, err
		}
		out = append(out, constraint)
	}
	return out, nil
}
