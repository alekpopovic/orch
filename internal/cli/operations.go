package cli

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	versioninfo "github.com/alekpopovic/orch/internal/version"
	"github.com/alekpopovic/orch/pkg/types"
	"github.com/spf13/cobra"
)

func (a *app) maintenanceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "maintenance", Short: "Manage maintenance windows"}
	var schedule, tz, operations string
	var duration time.Duration
	var global, enabled bool
	create := &cobra.Command{Use: "create <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(MaintenanceClient)
		if !ok {
			return fmt.Errorf("client does not support maintenance windows")
		}
		ops := []types.MaintenanceOperation{}
		for _, v := range strings.Split(operations, ",") {
			if strings.TrimSpace(v) != "" {
				ops = append(ops, types.MaintenanceOperation(strings.TrimSpace(v)))
			}
		}
		item, err := c.CreateMaintenanceWindow(cmd.Context(), types.MaintenanceWindow{Name: args[0], Schedule: schedule, Timezone: tz, Duration: duration, Global: global, Enabled: enabled, AllowedOperations: ops})
		if err != nil {
			return err
		}
		return writeValue(a.out, "json", item)
	}}
	create.Flags().StringVar(&schedule, "schedule", "", "five-field cron schedule")
	create.Flags().StringVar(&tz, "timezone", "UTC", "IANA timezone")
	create.Flags().DurationVar(&duration, "duration", time.Hour, "window duration")
	create.Flags().StringVar(&operations, "operations", "rollout,rollback,node_drain", "comma-separated allowed operations")
	create.Flags().BoolVar(&global, "global", false, "apply globally")
	create.Flags().BoolVar(&enabled, "enabled", true, "enable window")
	_ = create.MarkFlagRequired("schedule")
	cmd.AddCommand(create)
	cmd.AddCommand(&cobra.Command{Use: "ls", RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(MaintenanceClient)
		if !ok {
			return fmt.Errorf("client does not support maintenance windows")
		}
		v, err := c.ListMaintenanceWindows(cmd.Context())
		if err != nil {
			return err
		}
		return writeValue(a.out, "json", v)
	}})
	cmd.AddCommand(&cobra.Command{Use: "delete <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(MaintenanceClient)
		if !ok {
			return fmt.Errorf("client does not support maintenance windows")
		}
		if err := c.DeleteMaintenanceWindow(cmd.Context(), args[0]); err != nil {
			return err
		}
		fmt.Fprintf(a.out, "deleted maintenance window %s\n", args[0])
		return nil
	}})
	return cmd
}
func (a *app) retentionCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "retention", Short: "Inspect and run retention pruning"}
	cmd.AddCommand(&cobra.Command{Use: "status", RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.operationsClient()
		if err != nil {
			return err
		}
		v, err := c.GetRetentionStatus(cmd.Context())
		if err != nil {
			return err
		}
		return writeValue(a.out, "json", v)
	}})
	var dry bool
	prune := &cobra.Command{Use: "prune", RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := a.operationsClient()
		if err != nil {
			return err
		}
		v, err := c.PruneRetention(cmd.Context(), dry)
		if err != nil {
			return err
		}
		return writeValue(a.out, "json", v)
	}}
	prune.Flags().BoolVar(&dry, "dry-run", false, "count without deleting")
	cmd.AddCommand(prune)
	return cmd
}
func (a *app) usageCommand() *cobra.Command {
	var fromRaw, toRaw string
	load := func(cmd *cobra.Command) (types.UsageReport, error) {
		c, err := a.operationsClient()
		if err != nil {
			return types.UsageReport{}, err
		}
		from, err := parseCLIDate(fromRaw)
		if err != nil {
			return types.UsageReport{}, err
		}
		to, err := parseCLIDate(toRaw)
		if err != nil {
			return types.UsageReport{}, err
		}
		return c.GetUsageReport(cmd.Context(), a.namespace(), from, to)
	}
	cmd := &cobra.Command{Use: "usage", Short: "Show namespace usage", RunE: func(cmd *cobra.Command, _ []string) error {
		v, err := load(cmd)
		if err != nil {
			return err
		}
		return writeValue(a.out, "json", v)
	}}
	cmd.PersistentFlags().StringVar(&fromRaw, "from", "", "start date or RFC3339")
	cmd.PersistentFlags().StringVar(&toRaw, "to", "", "end date or RFC3339")
	var format string
	export := &cobra.Command{Use: "export", RunE: func(cmd *cobra.Command, _ []string) error {
		if format != "csv" {
			return fmt.Errorf("--format must be csv")
		}
		v, err := load(cmd)
		if err != nil {
			return err
		}
		writer := csv.NewWriter(a.out)
		_ = writer.Write([]string{"namespace", "timestamp", "cpu_millicores", "memory_bytes", "replicas", "services", "task_runtime_seconds", "public_ports", "storage_claims"})
		for _, x := range v.Snapshots {
			_ = writer.Write([]string{x.Namespace, x.Timestamp.Format(time.RFC3339), strconv.FormatInt(x.CPUMillicores, 10), strconv.FormatInt(x.MemoryBytes, 10), strconv.Itoa(x.Replicas), strconv.Itoa(x.Services), strconv.FormatFloat(x.TaskRuntimeSeconds, 'f', 3, 64), strconv.Itoa(x.PublicPorts), strconv.Itoa(x.StorageClaims)})
		}
		writer.Flush()
		return writer.Error()
	}}
	export.Flags().StringVar(&format, "format", "csv", "export format")
	cmd.AddCommand(export)
	return cmd
}
func (a *app) clusterCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "cluster", Short: "Cluster operations"}
	cmd.AddCommand(&cobra.Command{Use: "check-upgrade", RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(OperationsClient)
		if !ok {
			return fmt.Errorf("client does not support cluster operations")
		}
		v, err := c.GetVersion(cmd.Context())
		if err != nil {
			return err
		}
		nodes, err := client.ListNodes(cmd.Context())
		if err != nil {
			return err
		}
		status := "compatible"
		warnings := []string{}
		for _, node := range nodes {
			compatibility, checkErr := versioninfo.CheckAgent(node.AgentVersion)
			if checkErr != nil || compatibility == versioninfo.TooOld {
				status = "blocked"
				warnings = append(warnings, fmt.Sprintf("node %s has incompatible agent version %q", node.ID, node.AgentVersion))
			} else if compatibility == versioninfo.UntestedNewer {
				warnings = append(warnings, fmt.Sprintf("node %s agent version %q is newer than tested", node.ID, node.AgentVersion))
			}
		}
		return writeValue(a.out, "json", map[string]any{"status": status, "version": v, "warnings": warnings})
	}})
	return cmd
}
func (a *app) operationsClient() (OperationsClient, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	c, ok := client.(OperationsClient)
	if !ok {
		return nil, fmt.Errorf("client does not support cluster operations")
	}
	return c, nil
}
func parseCLIDate(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", v)
}
