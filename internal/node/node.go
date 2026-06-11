package node

import "github.com/alekpopovic/orch/pkg/types"

type Status string

const (
	StatusUnknown  Status = "unknown"
	StatusReady    Status = "ready"
	StatusDraining Status = "draining"
)

type Record struct {
	Node   types.Node
	Status Status
}
