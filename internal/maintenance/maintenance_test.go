package maintenance

import (
	"github.com/alekpopovic/orch/pkg/types"
	"testing"
	"time"
)

func TestAllowsWithFakeTime(t *testing.T) {
	w := types.MaintenanceWindow{Name: "night", Schedule: "0 2 * * *", Timezone: "UTC", Duration: time.Hour, Enabled: true, AllowedOperations: []types.MaintenanceOperation{types.MaintenanceRollout}}
	if err := Validate(w); err != nil {
		t.Fatal(err)
	}
	if !Allows(w, types.MaintenanceRollout, time.Date(2026, 8, 21, 2, 30, 0, 0, time.UTC)) {
		t.Fatal("active window rejected")
	}
	if Allows(w, types.MaintenanceRollout, time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC)) {
		t.Fatal("closed window allowed")
	}
}
