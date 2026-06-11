package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/alekpopovic/orch/pkg/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const version = "dev"

type Client interface {
	ListNodes(ctx context.Context) ([]types.Node, error)
	GetNode(ctx context.Context, id string) (types.Node, error)
	DrainNode(ctx context.Context, id string) (types.Node, error)
	UncordonNode(ctx context.Context, id string) (types.Node, error)
	CreateService(ctx context.Context, spec types.ServiceSpec) (types.Service, error)
	ListServices(ctx context.Context) ([]types.Service, error)
	GetService(ctx context.Context, id string) (types.Service, error)
	DeleteService(ctx context.Context, id string) error
	ScaleService(ctx context.Context, id string, replicas int) (types.Service, error)
	RolloutService(ctx context.Context, id string, image string) (types.Deployment, error)
	RollbackService(ctx context.Context, id string) (types.Deployment, error)
	ListTasks(ctx context.Context, query url.Values) ([]types.Task, error)
	GetTask(ctx context.Context, id string) (types.Task, error)
	ListEvents(ctx context.Context) ([]types.Event, error)
	StreamLogs(ctx context.Context, serviceID string, taskID string, follow bool, tail string, out io.Writer) error
}

type Config struct {
	ServerURL string `yaml:"server_url" json:"server_url"`
}

type Options struct {
	Out           io.Writer
	Err           io.Writer
	NewClient     func(serverURL string) (Client, error)
	DefaultConfig string
}

type app struct {
	out        io.Writer
	serverFlag string
	configPath string
	output     string
	newClient  func(serverURL string) (Client, error)
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
	root.PersistentFlags().StringVar(&a.output, "output", "table", "output format: table or json")
	root.PersistentFlags().StringVar(&a.configPath, "config", a.configPath, "CLI config file")

	root.AddCommand(a.versionCommand())
	root.AddCommand(a.nodeCommand())
	root.AddCommand(a.deployCommand())
	root.AddCommand(a.serviceCommand())
	root.AddCommand(a.scaleCommand())
	root.AddCommand(a.rolloutCommand())
	root.AddCommand(a.rollbackCommand())
	root.AddCommand(a.deleteCommand())
	root.AddCommand(a.eventsCommand())
	root.AddCommand(a.logsCommand())
	return root
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
			deployment, err := client.RolloutService(cmd.Context(), string(service.ID), image)
			if err != nil {
				return err
			}
			return writeDeployment(a.out, a.output, deployment)
		},
	}
	cmd.Flags().StringVar(&image, "image", "", "new image")
	_ = cmd.MarkFlagRequired("image")
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

func (a *app) eventsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "events",
		Short: "List events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			events, err := client.ListEvents(cmd.Context())
			if err != nil {
				return err
			}
			return writeEvents(a.out, a.output, events)
		},
	}
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
	return a.newClient(serverURL)
}

func (a *app) serverURL() (string, error) {
	if strings.TrimSpace(a.serverFlag) != "" {
		return strings.TrimSpace(a.serverFlag), nil
	}
	if env := strings.TrimSpace(os.Getenv("ORCH_SERVER_URL")); env != "" {
		return env, nil
	}
	cfg, err := readConfig(a.configPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfg.ServerURL) != "" {
		return strings.TrimSpace(cfg.ServerURL), nil
	}
	return "", fmt.Errorf("server URL is required; pass --server, set ORCH_SERVER_URL, or set server_url in %s", a.configPath)
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read config file %q: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config file %q: %w", path, err)
	}
	return cfg, nil
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
