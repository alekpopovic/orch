package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/alekpopovic/orch/pkg/types"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func (a *app) jobCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "job", Short: "Run and inspect finite jobs"}
	cmd.AddCommand(&cobra.Command{Use: "run <file.yaml>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		var manifest struct {
			Kind         string   `yaml:"kind"`
			Name         string   `yaml:"name"`
			Image        string   `yaml:"image"`
			Command      []string `yaml:"command"`
			BackoffLimit int      `yaml:"backoffLimit"`
			Resources    struct {
				CPU    string `yaml:"cpu"`
				Memory string `yaml:"memory"`
			} `yaml:"resources"`
		}
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return err
		}
		cpu, err := types.ParseCPU(manifest.Resources.CPU)
		if err != nil {
			return err
		}
		memory, err := types.ParseMemory(manifest.Resources.Memory)
		if err != nil {
			return err
		}
		spec := types.JobSpec{Name: manifest.Name, Image: manifest.Image, Command: manifest.Command, BackoffLimit: manifest.BackoffLimit, ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: cpu, Memory: memory}, Limits: types.Resources{CPU: cpu, Memory: memory}}}
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(JobClient)
		if !ok {
			return fmt.Errorf("client does not support jobs")
		}
		job, err := c.CreateJob(cmd.Context(), spec)
		if err != nil {
			return err
		}
		return writeJobs(a.out, a.output, []types.Job{job})
	}})
	cmd.AddCommand(&cobra.Command{Use: "ls", RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(JobClient)
		if !ok {
			return fmt.Errorf("client does not support jobs")
		}
		v, err := c.ListJobs(cmd.Context())
		if err != nil {
			return err
		}
		return writeJobs(a.out, a.output, v)
	}})
	cmd.AddCommand(&cobra.Command{Use: "logs <job>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(JobClient)
		if !ok {
			return fmt.Errorf("client does not support jobs")
		}
		job, err := c.GetJob(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if len(job.TaskIDs) == 0 {
			return fmt.Errorf("job has no task")
		}
		return c.StreamLogs(cmd.Context(), "", string(job.TaskIDs[len(job.TaskIDs)-1]), false, "all", a.out)
	}})
	return cmd
}

func (a *app) cronJobCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "cronjob", Short: "Manage scheduled jobs"}
	cmd.AddCommand(&cobra.Command{Use: "apply <file.yaml>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		var envelope struct {
			Kind              string                  `yaml:"kind"`
			Name              string                  `yaml:"name"`
			Schedule          string                  `yaml:"schedule"`
			Timezone          string                  `yaml:"timezone"`
			ConcurrencyPolicy types.ConcurrencyPolicy `yaml:"concurrencyPolicy"`
			JobTemplate       struct {
				Image     string   `yaml:"image"`
				Command   []string `yaml:"command"`
				Resources struct {
					CPU    string `yaml:"cpu"`
					Memory string `yaml:"memory"`
				} `yaml:"resources"`
			} `yaml:"jobTemplate"`
		}
		if err := yaml.Unmarshal(data, &envelope); err != nil {
			return err
		}
		cpu, err := types.ParseCPU(envelope.JobTemplate.Resources.CPU)
		if err != nil {
			return err
		}
		mem, err := types.ParseMemory(envelope.JobTemplate.Resources.Memory)
		if err != nil {
			return err
		}
		spec := types.CronJobSpec{Name: envelope.Name, Schedule: envelope.Schedule, Timezone: envelope.Timezone, ConcurrencyPolicy: envelope.ConcurrencyPolicy, JobTemplate: types.JobSpec{Name: envelope.Name, Image: envelope.JobTemplate.Image, Command: envelope.JobTemplate.Command, ResourceRequirements: types.ResourceRequirements{Requests: types.Resources{CPU: cpu, Memory: mem}, Limits: types.Resources{CPU: cpu, Memory: mem}}}}
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(CronJobClient)
		if !ok {
			return fmt.Errorf("client does not support cronjobs")
		}
		v, err := c.CreateCronJob(cmd.Context(), spec)
		if err != nil {
			return err
		}
		return writeCronJobs(a.out, a.output, []types.CronJob{v})
	}})
	cmd.AddCommand(&cobra.Command{Use: "ls", RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(CronJobClient)
		if !ok {
			return fmt.Errorf("client does not support cronjobs")
		}
		v, err := c.ListCronJobs(cmd.Context())
		if err != nil {
			return err
		}
		return writeCronJobs(a.out, a.output, v)
	}})
	for _, value := range []bool{true, false} {
		suspended := value
		action := "resume"
		if suspended {
			action = "suspend"
		}
		cmd.AddCommand(&cobra.Command{Use: action + " <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			client, err := a.client()
			if err != nil {
				return err
			}
			c, ok := client.(CronJobClient)
			if !ok {
				return fmt.Errorf("client does not support cronjobs")
			}
			v, err := c.SetCronJobSuspended(cmd.Context(), args[0], suspended)
			if err != nil {
				return err
			}
			return writeCronJobs(a.out, a.output, []types.CronJob{v})
		}})
	}
	return cmd
}

func (a *app) volumeCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "volume", Short: "Manage persistent volumes"}
	cmd.AddCommand(&cobra.Command{Use: "ls", RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(VolumeClient)
		if !ok {
			return fmt.Errorf("client does not support volumes")
		}
		v, err := c.ListVolumes(cmd.Context())
		if err != nil {
			return err
		}
		return writeVolumes(a.out, a.output, v)
	}})
	var node string
	create := &cobra.Command{Use: "create <name>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(VolumeClient)
		if !ok {
			return fmt.Errorf("client does not support volumes")
		}
		v, err := c.CreateVolume(cmd.Context(), types.Volume{Name: strings.TrimSpace(args[0]), Driver: "local", NodeID: types.NodeID(node)})
		if err != nil {
			return err
		}
		return writeVolumes(a.out, a.output, []types.Volume{v})
	}}
	create.Flags().StringVar(&node, "node", "", "pin local volume to node")
	cmd.AddCommand(create)
	cmd.AddCommand(&cobra.Command{Use: "inspect <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := a.client()
		if err != nil {
			return err
		}
		c, ok := client.(VolumeClient)
		if !ok {
			return fmt.Errorf("client does not support volumes")
		}
		v, err := c.GetVolume(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		return writeVolumes(a.out, a.output, []types.Volume{v})
	}})
	return cmd
}
