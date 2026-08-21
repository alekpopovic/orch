package dns

import (
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/pkg/types"
)

type fakeStore struct {
	services []types.Service
	tasks    []types.Task
	nodes    []types.Node
}

func (f fakeStore) ListServices(context.Context) ([]types.Service, error) { return f.services, nil }
func (f fakeStore) ListTasks(context.Context, controlplane.TaskFilter) ([]types.Task, error) {
	return f.tasks, nil
}
func (f fakeStore) ListNodes(context.Context) ([]types.Node, error) { return f.nodes, nil }
func query(name string) []byte {
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[0:2], 7)
	binary.BigEndian.PutUint16(packet[4:6], 1)
	for _, label := range split(name) {
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	packet = append(packet, 0, 0, 1, 0, 1)
	return packet
}
func split(v string) []string {
	out := []string{}
	start := 0
	for i := 0; i <= len(v); i++ {
		if i == len(v) || v[i] == '.' {
			out = append(out, v[start:i])
			start = i + 1
		}
	}
	return out
}

func TestResolveHealthyEndpointsAndDNSPacket(t *testing.T) {
	metrics := &Metrics{}
	store := fakeStore{services: []types.Service{{ID: "service", Namespace: "payments", Status: types.ServiceActive, Spec: types.ServiceSpec{Name: "api"}}}, tasks: []types.Task{{ServiceID: "service", NodeID: "node", ActualStatus: types.TaskHealthy}}, nodes: []types.Node{{ID: "node", AdvertiseAddress: "10.0.0.8:7443"}}}
	server := New(store, 15*time.Second, "default", metrics)
	ips, err := server.Resolve(context.Background(), "api.payments.svc.orch")
	if err != nil {
		t.Fatal(err)
	}
	if len(ips) != 1 || ips[0].String() != "10.0.0.8" {
		t.Fatalf("ips=%v", ips)
	}
	response, err := server.ServePacket(context.Background(), query("api.payments.svc.orch"))
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint16(response[6:8]); got != 1 {
		t.Fatalf("answers=%d", got)
	}
	if metrics.Queries() != 2 {
		t.Fatalf("queries=%d", metrics.Queries())
	}
	if text := metrics.Prometheus(); text == "" {
		t.Fatal("missing metrics")
	}
}

func TestResolveRejectsUnsupportedName(t *testing.T) {
	metrics := &Metrics{}
	_, err := New(fakeStore{}, time.Second, "default", metrics).Resolve(context.Background(), "example.com")
	if err == nil || metrics.Errors() != 1 {
		t.Fatalf("err=%v errors=%d", err, metrics.Errors())
	}
}
