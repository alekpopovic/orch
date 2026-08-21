package maintenance

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/alekpopovic/orch/internal/cronjobs"
	"github.com/alekpopovic/orch/pkg/types"
)

type forceKey struct{}

func WithForce(ctx context.Context) context.Context { return context.WithValue(ctx, forceKey{}, true) }
func Forced(ctx context.Context) bool               { v, _ := ctx.Value(forceKey{}).(bool); return v }
func Validate(window types.MaintenanceWindow) error {
	if window.Name == "" {
		return fmt.Errorf("name is required")
	}
	if _, err := cronjobs.Parse(window.Schedule); err != nil {
		return err
	}
	if window.Timezone == "" {
		window.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(window.Timezone); err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	if window.Duration <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if len(window.AllowedOperations) == 0 {
		return fmt.Errorf("allowed operations are required")
	}
	valid := []types.MaintenanceOperation{
		types.MaintenanceRollout,
		types.MaintenanceRollback,
		types.MaintenanceNodeDrain,
		types.MaintenanceScaleDown,
		types.MaintenanceReplacement,
	}
	for _, operation := range window.AllowedOperations {
		if !slices.Contains(valid, operation) {
			return fmt.Errorf("unsupported operation %q", operation)
		}
	}
	return nil
}
func Allows(window types.MaintenanceWindow, operation types.MaintenanceOperation, now time.Time) bool {
	if !window.Enabled || !slices.Contains(window.AllowedOperations, operation) {
		return false
	}
	loc, err := time.LoadLocation(window.Timezone)
	if err != nil {
		return false
	}
	schedule, err := cronjobs.Parse(window.Schedule)
	if err != nil {
		return false
	}
	localized := now.In(loc)
	return len(schedule.Between(localized.Add(-window.Duration), localized, 1000)) > 0
}
