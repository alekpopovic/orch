package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/pkg/types"
)

func writeValue(w io.Writer, output string, value any) error {
	if output == "json" {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	return fmt.Errorf("table output for %T is not implemented", value)
}

func writeNodes(w io.Writer, output string, nodes []types.Node) error {
	if output == "json" {
		return writeValue(w, output, nodes)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tHOSTNAME\tADDRESS\tSTATUS\tCPU\tMEMORY")
	for _, node := range nodes {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\n", node.ID, node.Hostname, node.AdvertiseAddress, node.Status, node.Capacity.CPU, node.Capacity.Memory)
	}
	return tw.Flush()
}

func writeNode(w io.Writer, output string, node types.Node) error {
	if output == "json" {
		return writeValue(w, output, node)
	}
	return writeNodes(w, output, []types.Node{node})
}

func writeServices(w io.Writer, output string, services []types.Service) error {
	if output == "json" {
		return writeValue(w, output, services)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tIMAGE\tREPLICAS\tVERSION")
	for _, service := range services {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\n", service.ID, service.Spec.Name, service.Spec.Image, service.Spec.Replicas, service.DeploymentVersion)
	}
	return tw.Flush()
}

func writeService(w io.Writer, output string, service types.Service) error {
	if output == "json" {
		return writeValue(w, output, service)
	}
	return writeServices(w, output, []types.Service{service})
}

func writeDeployment(w io.Writer, output string, deployment types.Deployment) error {
	if output == "json" {
		return writeValue(w, output, deployment)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSERVICE\tFROM\tTO\tSTRATEGY\tSTATUS")
	fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%s\t%s\n", deployment.ID, deployment.ServiceID, deployment.FromVersion, deployment.ToVersion, deployment.Strategy, deployment.Status)
	return tw.Flush()
}

func writeTasks(w io.Writer, output string, tasks []types.Task) error {
	if output == "json" {
		return writeValue(w, output, tasks)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSERVICE\tNODE\tDESIRED\tACTUAL\tIMAGE\tRESTARTS")
	for _, task := range tasks {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n", task.ID, task.ServiceID, task.NodeID, task.DesiredStatus, task.ActualStatus, task.Image, task.RestartCount)
	}
	return tw.Flush()
}

func writeEndpoints(w io.Writer, output string, endpoints discovery.ServiceEndpoints) error {
	if output == "json" {
		return writeValue(w, output, endpoints)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SERVICE\tTASK\tNODE\tADDRESS\tHOST_PORT\tCONTAINER_PORT\tPROTO\tHEALTH\tVERSION")
	for _, endpoint := range endpoints.Endpoints {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\t%d\n",
			endpoint.ServiceName,
			endpoint.TaskID,
			endpoint.NodeID,
			endpoint.NodeAddress,
			endpoint.PublicHostPort,
			endpoint.ContainerPort,
			endpoint.Protocol,
			endpoint.HealthStatus,
			endpoint.ServiceVersion,
		)
	}
	return tw.Flush()
}

func writeEvents(w io.Writer, output string, events []types.Event) error {
	if output == "json" {
		return writeValue(w, output, events)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TIME\tSEVERITY\tTYPE\tSOURCE\tOBJECT\tMESSAGE")
	for _, event := range events {
		object := strings.Trim(event.RelatedObjectType+"/"+event.RelatedObjectID, "/")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", event.Timestamp.Format("2006-01-02T15:04:05Z07:00"), event.Severity, event.Type, event.Source, object, event.Message)
	}
	return tw.Flush()
}
