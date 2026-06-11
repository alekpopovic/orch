package store

import (
	"context"

	"github.com/alekpopovic/orch/pkg/types"
)

type Store interface {
	SaveNode(ctx context.Context, node types.Node) error
	SaveTask(ctx context.Context, task types.Task) error
}
