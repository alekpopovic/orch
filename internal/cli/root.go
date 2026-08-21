package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alekpopovic/orch/internal/audit"
	"github.com/alekpopovic/orch/internal/config"
	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/internal/events"
	"github.com/alekpopovic/orch/pkg/types"
	"github.com/spf13/cobra"
)

const version = "dev"

type Client interface {
	ListNodes(ctx context.Context) ([]types.Node, error)
	GetNode(ctx context.Context, id string) (types.Node, error)
	DrainNode(ctx context.Context, id string) (types.Node, error)
	UncordonNode(ctx context.Context, id string) (types.Node, error)
	GetNodeDrainStatus(ctx context.Context, id string) (controlplane.NodeDrainStatus, error)
	CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error)
	ListServices(ctx context.Context) ([]types.Service, error)
	GetService(ctx context.Context, id string) (types.Service, error)
	GetServiceEndpoints(ctx context.Context, id string, includeUnhealthy bool) (discovery.ServiceEndpoints, error)
	DeleteService(ctx context.Context, id string) error
	ScaleService(ctx context.Context, id string, replicas int) (types.Service, error)
	RolloutService(ctx context.Context, id string, image string, maxUnavailable int, maxSurge int) (types.Deployment, error)
	GetServiceRollout(ctx context.Context, id string) (types.Deployment, error)
	RollbackService(ctx context.Context, id string) (types.Deployment, error)
	ListTasks(ctx context.Context, query url.Values) ([]types.Task, error)
	GetTask(ctx context.Context, id string) (types.Task, error)
	ListEvents(ctx context.Context, filter events.Filter) ([]types.Event, error)
	ListAuditLogs(ctx context.Context, filter audit.Filter) ([]audit.Log, error)
	StreamLogs(ctx context.Context, serviceID string, taskID string, follow bool, tail string, out io.Writer) error
}

type Config = config.CLIConfig

type NamespaceClient interface {
	CreateNamespace(ctx context.Context, name string) (types.Namespace, error)
	ListNamespaces(ctx context.Context) ([]types.Namespace, error)
	DeleteNamespace(ctx context.Context, name string) error
}

type QuotaClient interface {
	GetResourceQuota(context.Context) (types.ResourceQuota, types.ResourceUsage, error)
	SetResourceQuota(context.Context, types.ResourceQuota) (types.ResourceQuota, types.ResourceUsage, error)
}

type GitOpsClient interface {
	CreateGitOpsSource(context.Context, types.GitOpsSource) (types.GitOpsSource, error)
	ListGitOpsSources(context.Context) ([]types.GitOpsSource, error)
	SyncGitOpsSource(context.Context, string) (types.GitOpsSource, error)
	DeleteGitOpsSource(context.Context, string) error
}

type Options struct {
	Out           io.Writer
	Err           io.Writer
	NewClient     func(serverURL string) (Client, error)
	DefaultConfig string
}

type app struct {
	out           io.Writer
	serverFlag    string
	tokenFlag     string
	namespaceFlag string
	configPath    string
	output        string
	newClient     func(serverURL string) (Client, error)
}

func NewRootCommand(opts Options) *cobra.Command {
	out := opts.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.Err
	if errOut == nil {
		errOut = os.Stderr
	}
	newClient := opts.NewClient
	if newClient == nil {
		newClient = func(serverURL string) (Client, error) {
			return NewAPIClient(serverURL)
		}
	}

	a := &app{
		out:        out,
		output:     "table",
		configPath: defaultConfigPath(),
		newClient:  newClient,
	}
	if opts.DefaultConfig != "" {
		a.configPath = opts.DefaultConfig
	}

	root := &cobra.Command{
		Use:           "orch",
		Short:         "CLI for the orch container orchestrator",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if a.output != "table" && a.output != "json" {
				return fmt.Errorf("--output must be table or json")
			}
			return nil
		},
	}
	root.SetOut(out)
	root.SetErr(errOut)
	root.PersistentFlags().StringVar(&a.serverFlag, "server", "", "control-plane server URL")
	root.PersistentFlags().StringVar(&a.tokenFlag, "token", "", "user API JWT bearer token")
	root.PersistentFlags().StringVarP(&a.namespaceFlag, "namespace", "n", "", "workload namespace")
	root.PersistentFlags().StringVar(&a.output, "output", "table", "output format: table or json")
	root.PersistentFlags().StringVar(&a.configPath, "config", a.configPath, "CLI config file")

	root.AddCommand(a.versionCommand())
	root.AddCommand(a.validateCommand())
	root.AddCommand(a.nodeCommand())
	root.AddCommand(a.deployCommand())
	root.AddCommand(a.serviceCommand())
	root.AddCommand(a.scaleCommand())
	root.AddCommand(a.rolloutCommand())
	root.AddCommand(a.rollbackCommand())
	root.AddCommand(a.deleteCommand())
	root.AddCommand(a.endpointsCommand())
	root.AddCommand(a.eventsCommand())
	root.AddCommand(a.auditCommand())
	root.AddCommand(a.logsCommand())
	root.AddCommand(a.namespaceCommand())
	root.AddCommand(a.quotaCommand())
	root.AddCommand(a.gitopsCommand())
	return root
}

func (a *app) namespaceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "namespace", Short: "Manage namespaces"}
	cmd.AddCommand(&cobra.Command{
		Use: "ls", Short: "List namespaces",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			namespaceClient, ok := client.(NamespaceClient)
			if !ok {
				return fmt.Errorf("client does not support namespaces")
			}
			items, err := namespaceClient.ListNamespaces(cmd.Context())
			if err != nil {
				return err
			}
			return writeNamespaces(a.out, a.output, items)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "create <name>", Short: "Create a namespace", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			namespaceClient, ok := client.(NamespaceClient)
			if !ok {
				return fmt.Errorf("client does not support namespaces")
			}
			item, err := namespaceClient.CreateNamespace(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeNamespaces(a.out, a.output, []types.Namespace{item})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "delete <name>", Short: "Delete an empty namespace", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			namespaceClient, ok := client.(NamespaceClient)
			if !ok {
				return fmt.Errorf("client does not support namespaces")
			}
			if err := namespaceClient.DeleteNamespace(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(a.out, "deleted namespace %s\n", args[0])
			return nil
		},
	})
	return cmd
}

func (a *app) quotaCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "quota", Short: "Manage namespace resource quotas"}
	cmd.AddCommand(&cobra.Command{
		Use: "get", Short: "Show quota and current usage",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			quotaClient, ok := client.(QuotaClient)
			if !ok {
				return fmt.Errorf("client does not support quotas")
			}
			value, usage, err := quotaClient.GetResourceQuota(cmd.Context())
			if err != nil {
				return err
			}
			return writeQuota(a.out, a.output, value, usage)
		},
	})
	var value types.ResourceQuota
	set := &cobra.Command{
		Use: "set", Short: "Set quota limits (zero means unlimited)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			quotaClient, ok := client.(QuotaClient)
			if !ok {
				return fmt.Errorf("client does not support quotas")
			}
			updated, usage, err := quotaClient.SetResourceQuota(cmd.Context(), value)
			if err != nil {
				return err
			}
			return writeQuota(a.out, a.output, updated, usage)
		},
	}
	set.Flags().IntVar(&value.MaxServices, "max-services", 0, "maximum services")
	set.Flags().IntVar(&value.MaxReplicas, "max-replicas", 0, "maximum replicas")
	set.Flags().Int64Var(&value.MaxCPUMillicores, "max-cpu-millicores", 0, "maximum requested CPU millicores")
	set.Flags().Int64Var(&value.MaxMemoryBytes, "max-memory-bytes", 0, "maximum requested memory bytes")
	set.Flags().IntVar(&value.MaxPublicPorts, "max-public-ports", 0, "maximum public ports")
	set.Flags().IntVar(&value.MaxSecrets, "max-secrets", 0, "maximum secrets")
	set.Flags().IntVar(&value.MaxRegistryCredentials, "max-registry-credentials", 0, "maximum registry credentials")
	cmd.AddCommand(set)
	return cmd
}

func (a *app) gitopsCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "gitops", Short: "Manage GitOps sources"}
	var repositoryURL, branch, sourcePath string
	var interval time.Duration
	var prune bool
	add := &cobra.Command{
		Use: "add", Short: "Add a GitOps source",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			gitopsClient, ok := client.(GitOpsClient)
			if !ok {
				return fmt.Errorf("client does not support GitOps")
			}
			source, err := gitopsClient.CreateGitOpsSource(cmd.Context(), types.GitOpsSource{RepositoryURL: repositoryURL, Branch: branch, Path: sourcePath, SyncInterval: interval, Prune: prune})
			if err != nil {
				return err
			}
			return writeGitOpsSources(a.out, a.output, []types.GitOpsSource{source})
		},
	}
	add.Flags().StringVar(&repositoryURL, "repo", "", "Git repository URL")
	add.Flags().StringVar(&branch, "branch", "main", "Git branch")
	add.Flags().StringVar(&sourcePath, "path", ".", "manifest file or directory")
	add.Flags().DurationVar(&interval, "interval", time.Minute, "sync interval")
	add.Flags().BoolVar(&prune, "prune", false, "delete services whose manifests are removed")
	_ = add.MarkFlagRequired("repo")
	cmd.AddCommand(add)
	cmd.AddCommand(&cobra.Command{
		Use: "ls", Short: "List GitOps sources",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			gitopsClient, ok := client.(GitOpsClient)
			if !ok {
				return fmt.Errorf("client does not support GitOps")
			}
			sources, err := gitopsClient.ListGitOpsSources(cmd.Context())
			if err != nil {
				return err
			}
			return writeGitOpsSources(a.out, a.output, sources)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "sync <id>", Short: "Synchronize a GitOps source now", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			gitopsClient, ok := client.(GitOpsClient)
			if !ok {
				return fmt.Errorf("client does not support GitOps")
			}
			source, err := gitopsClient.SyncGitOpsSource(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeGitOpsSources(a.out, a.output, []types.GitOpsSource{source})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use: "delete <id>", Short: "Delete a GitOps source", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			gitopsClient, ok := client.(GitOpsClient)
			if !ok {
				return fmt.Errorf("client does not support GitOps")
			}
			if err := gitopsClient.DeleteGitOpsSource(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(a.out, "deleted GitOps source %s\n", args[0])
			return nil
		},
	})
	return cmd
}

func Execute(ctx context.Context, args []string, out io.Writer, errOut io.Writer) error {
	cmd := NewRootCommand(Options{Out: out, Err: errOut})
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

func (a *app) versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(a.out, "orch "+version)
			return nil
		},
	}
}

func (a *app) validateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <file.yaml>",
		Short: "Validate a service deployment YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			spec, err := ParseDeployFile(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(a.out, "service spec %q is valid\n", spec.Name)
			return nil
		},
	}
}

func (a *app) nodeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "node", Short: "Manage nodes"}
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List nodes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			nodes, err := client.ListNodes(cmd.Context())
			if err != nil {
				return err
			}
			return writeNodes(a.out, a.output, nodes)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "inspect <node-id>",
		Short: "Inspect a node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			node, err := client.GetNode(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeNode(a.out, a.output, node)
		},
	})
	cmd.AddCommand(a.nodeActionCommand("drain", "Drain a node", func(ctx context.Context, client Client, id string) (types.Node, error) {
		return client.DrainNode(ctx, id)
	}))
	cmd.AddCommand(a.nodeActionCommand("uncordon", "Uncordon a node", func(ctx context.Context, client Client, id string) (types.Node, error) {
		return client.UncordonNode(ctx, id)
	}))
	cmd.AddCommand(&cobra.Command{
		Use:   "drain-status <node-id>",
		Short: "Show node drain status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			status, err := client.GetNodeDrainStatus(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return writeNodeDrainStatus(a.out, a.output, status)
		},
	})
	return cmd
}

func (a *app) nodeActionCommand(name string, short string, action func(context.Context, Client, string) (types.Node, error)) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <node-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			node, err := action(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(a.out, "node %s is %s\n", node.ID, node.Status)
			return nil
		},
	}
}

func (a *app) deployCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deploy <file.yaml>",
		Short: "Deploy a service from YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := ParseDeployFile(args[0])
			if err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			service, err := client.CreateService(cmd.Context(), spec)
			if err != nil {
				return err
			}
			return writeServices(a.out, a.output, []types.Service{service})
		},
	}
}

func (a *app) serviceCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Manage services"}
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List services",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			services, err := client.ListServices(cmd.Context())
			if err != nil {
				return err
			}
			return writeServices(a.out, a.output, services)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "inspect <service-name-or-id>",
		Short: "Inspect a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			service, err := a.resolveService(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}
			return writeService(a.out, a.output, service)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "ps <service-name-or-id>",
		Short: "List service tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			service, err := a.resolveService(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}
			query := url.Values{"service_id": []string{string(service.ID)}}
			tasks, err := client.ListTasks(cmd.Context(), query)
			if err != nil {
				return err
			}
			return writeTasks(a.out, a.output, tasks)
		},
	})
	return cmd
}

func (a *app) scaleCommand() *cobra.Command {
	var replicas int
	cmd := &cobra.Command{
		Use:   "scale <service-name-or-id>",
		Short: "Scale a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if replicas < 0 {
				return fmt.Errorf("replicas must be zero or greater")
			}
			client, service, err := a.clientAndService(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			scaled, err := client.ScaleService(cmd.Context(), string(service.ID), replicas)
			if err != nil {
				return err
			}
			return writeServices(a.out, a.output, []types.Service{scaled})
		},
	}
	cmd.Flags().IntVar(&replicas, "replicas", -1, "desired replica count")
	_ = cmd.MarkFlagRequired("replicas")
	return cmd
}

func (a *app) rolloutCommand() *cobra.Command {
	var image string
	var maxUnavailable int
	var maxSurge int
	cmd := &cobra.Command{
		Use:   "rollout <service-name-or-id>",
		Short: "Roll out a new service image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			image = strings.TrimSpace(image)
			if image == "" {
				return fmt.Errorf("--image is required")
			}
			client, service, err := a.clientAndService(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if maxUnavailable < 0 {
				return fmt.Errorf("--max-unavailable must be zero or greater")
			}
			if maxSurge < 0 {
				return fmt.Errorf("--max-surge must be zero or greater")
			}
			if maxUnavailable == 0 && maxSurge == 0 {
				return fmt.Errorf("--max-unavailable and --max-surge cannot both be zero")
			}
			deployment, err := client.RolloutService(cmd.Context(), string(service.ID), image, maxUnavailable, maxSurge)
			if err != nil {
				return err
			}
			return writeDeployment(a.out, a.output, deployment)
		},
	}
	cmd.Flags().StringVar(&image, "image", "", "new image")
	cmd.Flags().IntVar(&maxUnavailable, "max-unavailable", 1, "maximum unavailable replicas during rollout")
	cmd.Flags().IntVar(&maxSurge, "max-surge", 1, "maximum extra replicas during rollout")
	_ = cmd.MarkFlagRequired("image")
	cmd.AddCommand(&cobra.Command{
		Use:   "status <service-name-or-id>",
		Short: "Show latest rollout status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, service, err := a.clientAndService(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			deployment, err := client.GetServiceRollout(cmd.Context(), string(service.ID))
			if err != nil {
				return err
			}
			return writeDeployment(a.out, a.output, deployment)
		},
	})
	return cmd
}

func (a *app) rollbackCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <service-name-or-id>",
		Short: "Roll back a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, service, err := a.clientAndService(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			deployment, err := client.RollbackService(cmd.Context(), string(service.ID))
			if err != nil {
				return err
			}
			return writeDeployment(a.out, a.output, deployment)
		},
	}
}

func (a *app) deleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <service-name-or-id>",
		Short: "Delete a service",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, service, err := a.clientAndService(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := client.DeleteService(cmd.Context(), string(service.ID)); err != nil {
				return err
			}
			fmt.Fprintf(a.out, "deleted service %s\n", service.Spec.Name)
			return nil
		},
	}
}

func (a *app) endpointsCommand() *cobra.Command {
	var includeUnhealthy bool
	cmd := &cobra.Command{
		Use:   "endpoints <service-name-or-id>",
		Short: "List service discovery endpoints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, service, err := a.clientAndService(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			endpoints, err := client.GetServiceEndpoints(cmd.Context(), string(service.ID), includeUnhealthy)
			if err != nil {
				return err
			}
			return writeEndpoints(a.out, a.output, endpoints)
		},
	}
	cmd.Flags().BoolVar(&includeUnhealthy, "include-unhealthy", false, "include unhealthy task endpoints")
	return cmd
}

func (a *app) eventsCommand() *cobra.Command {
	var serviceRef string
	var follow bool
	var eventType string
	var severity string
	cmd := &cobra.Command{
		Use:   "events",
		Short: "List events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			filter := events.Filter{Type: strings.TrimSpace(eventType)}
			if severity != "" {
				filter.Severity = types.EventSeverity(strings.TrimSpace(severity))
			}
			if serviceRef != "" {
				service, err := a.resolveService(cmd.Context(), client, serviceRef)
				if err != nil {
					return err
				}
				filter.ServiceID = service.ID
			}
			return a.writeEventStream(cmd.Context(), client, filter, follow)
		},
	}
	cmd.Flags().StringVar(&serviceRef, "service", "", "filter by service name or ID")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow events")
	cmd.Flags().StringVar(&eventType, "type", "", "filter by event type")
	cmd.Flags().StringVar(&severity, "severity", "", "filter by severity: info, warning, error")
	return cmd
}

func (a *app) writeEventStream(ctx context.Context, client Client, filter events.Filter, follow bool) error {
	for {
		items, err := client.ListEvents(ctx, filter)
		if err != nil {
			return err
		}
		if err := writeEvents(a.out, a.output, items); err != nil {
			return err
		}
		if !follow {
			return nil
		}
		for _, event := range items {
			if event.Timestamp.After(filter.Since) {
				filter.Since = event.Timestamp
			}
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (a *app) auditCommand() *cobra.Command {
	var actorType string
	var actorID string
	var action string
	var targetType string
	var targetID string
	var outcome string
	var since string
	var limit int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "List audit logs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			filter := audit.Filter{
				ActorID:    strings.TrimSpace(actorID),
				Action:     strings.TrimSpace(action),
				TargetType: strings.TrimSpace(targetType),
				TargetID:   strings.TrimSpace(targetID),
				Limit:      limit,
			}
			if actorType != "" {
				filter.ActorType = audit.ActorType(strings.TrimSpace(actorType))
			}
			if outcome != "" {
				filter.Outcome = audit.Outcome(strings.TrimSpace(outcome))
			}
			if strings.TrimSpace(since) != "" {
				parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(since))
				if err != nil {
					return fmt.Errorf("--since must be RFC3339: %w", err)
				}
				filter.Since = parsed.UTC()
			}
			logs, err := client.ListAuditLogs(cmd.Context(), filter)
			if err != nil {
				return err
			}
			return writeAuditLogs(a.out, a.output, logs)
		},
	}
	cmd.Flags().StringVar(&actorType, "actor-type", "", "filter by actor type: user, agent, system")
	cmd.Flags().StringVar(&actorID, "actor-id", "", "filter by actor ID")
	cmd.Flags().StringVar(&action, "action", "", "filter by action")
	cmd.Flags().StringVar(&targetType, "target-type", "", "filter by target type")
	cmd.Flags().StringVar(&targetID, "target-id", "", "filter by target ID")
	cmd.Flags().StringVar(&outcome, "outcome", "", "filter by outcome: success, failure")
	cmd.Flags().StringVar(&since, "since", "", "filter since RFC3339 timestamp")
	cmd.Flags().IntVar(&limit, "limit", 100, "maximum audit records to return")
	return cmd
}

func (a *app) logsCommand() *cobra.Command {
	var follow bool
	var taskID string
	var tail string
	cmd := &cobra.Command{
		Use:   "logs <service-name-or-id>",
		Short: "Fetch service logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, service, err := a.clientAndService(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return client.StreamLogs(cmd.Context(), string(service.ID), taskID, follow, tail, a.out)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	cmd.Flags().StringVar(&taskID, "task", "", "specific task ID")
	cmd.Flags().StringVar(&tail, "tail", "", "number of lines to show from the end")
	return cmd
}

func (a *app) clientAndService(ctx context.Context, ref string) (Client, types.Service, error) {
	client, err := a.client()
	if err != nil {
		return nil, types.Service{}, err
	}
	service, err := a.resolveService(ctx, client, ref)
	if err != nil {
		return nil, types.Service{}, err
	}
	return client, service, nil
}

func (a *app) resolveService(ctx context.Context, client Client, ref string) (types.Service, error) {
	if validUUID(ref) {
		return client.GetService(ctx, ref)
	}
	services, err := client.ListServices(ctx)
	if err != nil {
		return types.Service{}, err
	}
	for _, service := range services {
		if service.Spec.Name == ref {
			return service, nil
		}
	}
	return types.Service{}, fmt.Errorf("service %q not found", ref)
}

func (a *app) client() (Client, error) {
	serverURL, err := a.serverURL()
	if err != nil {
		return nil, err
	}
	client, err := a.newClient(serverURL)
	if err != nil {
		return nil, err
	}
	if apiClient, ok := client.(*APIClient); ok {
		apiClient.SetToken(a.token())
	}
	if setter, ok := client.(interface{ SetNamespace(string) }); ok {
		setter.SetNamespace(a.namespace())
	}
	return client, nil
}

func (a *app) serverURL() (string, error) {
	cfg, err := config.LoadCLI(a.configPath, config.CLIOverrides{ServerURL: a.serverFlag})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfg.ServerURL) != "" {
		return strings.TrimSpace(cfg.ServerURL), nil
	}
	return "", fmt.Errorf("server URL is required; pass --server, set ORCH_SERVER_URL, or set server_url in %s", a.configPath)
}

func (a *app) token() string {
	cfg, err := config.LoadCLI(a.configPath, config.CLIOverrides{Token: a.tokenFlag})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Token)
}

func (a *app) namespace() string {
	cfg, err := config.LoadCLI(a.configPath, config.CLIOverrides{Namespace: a.namespaceFlag})
	if err != nil {
		return "default"
	}
	return strings.TrimSpace(cfg.Namespace)
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".orch.yaml"
	}
	return filepath.Join(dir, "orch", "config.yaml")
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
	}
	return true
}
