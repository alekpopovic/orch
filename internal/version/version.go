package version

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alekpopovic/orch/pkg/types"
)

const (
	Server             = "0.3.0"
	MinimumAgent       = "0.2.0"
	MaximumTestedAgent = "0.3.0"
	Schema             = 16
	MinimumSchema      = 15
	MaximumSchema      = 16
)

type Compatibility string

const (
	Compatible    Compatibility = "compatible"
	TooOld        Compatibility = "too_old"
	UntestedNewer Compatibility = "untested_newer"
)

func Info() types.VersionInfo {
	return types.VersionInfo{APIVersion: "v1", ServerVersion: Server, MinimumAgentVersion: MinimumAgent, MaximumTestedAgentVersion: MaximumTestedAgent, DatabaseSchemaVersion: Schema, MinimumSchemaVersion: MinimumSchema, MaximumSchemaVersion: MaximumSchema}
}
func CheckAgent(value string) (Compatibility, error) {
	if strings.TrimSpace(value) == "" {
		value = Server
	}
	v, err := parse(value)
	if err != nil {
		return "", err
	}
	min, _ := parse(MinimumAgent)
	max, _ := parse(MaximumTestedAgent)
	if compare(v, min) < 0 {
		return TooOld, nil
	}
	if compare(v, max) > 0 {
		return UntestedNewer, nil
	}
	return Compatible, nil
}
func CheckSchema(value int) error {
	if value < MinimumSchema {
		return fmt.Errorf("database schema %d is older than minimum supported %d", value, MinimumSchema)
	}
	if value > MaximumSchema {
		return fmt.Errorf("database schema %d is newer than maximum supported %d", value, MaximumSchema)
	}
	return nil
}
func parse(value string) ([3]int, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("version %q must use major.minor.patch", value)
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.SplitN(p, "-", 2)[0])
		if err != nil || n < 0 {
			return [3]int{}, fmt.Errorf("invalid version %q", value)
		}
		out[i] = n
	}
	return out, nil
}
func compare(a, b [3]int) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
