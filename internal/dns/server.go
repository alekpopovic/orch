package dns

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/internal/namespace"
	"github.com/alekpopovic/orch/pkg/types"
)

type Store interface {
	ListServices(context.Context) ([]types.Service, error)
	ListTasks(context.Context, controlplane.TaskFilter) ([]types.Task, error)
	ListNodes(context.Context) ([]types.Node, error)
}
type Metrics struct {
	queries atomic.Uint64
	errors  atomic.Uint64
}
type MetricsRecorder interface { IncDNSQuery(); IncDNSError() }
func (m *Metrics) IncDNSQuery() { m.queries.Add(1) }
func (m *Metrics) IncDNSError() { m.errors.Add(1) }

func (m *Metrics) Queries() uint64 { return m.queries.Load() }
func (m *Metrics) Errors() uint64  { return m.errors.Load() }
func (m *Metrics) Prometheus() string {
	return fmt.Sprintf("# TYPE dns_queries_total counter\ndns_queries_total %d\n# TYPE dns_errors_total counter\ndns_errors_total %d\n", m.Queries(), m.Errors())
}

type Server struct {
	store            Store
	ttl              uint32
	defaultNamespace string
	metrics          MetricsRecorder
}

func New(store Store, ttl time.Duration, defaultNamespace string, metrics MetricsRecorder) *Server {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if defaultNamespace == "" {
		defaultNamespace = namespace.Default
	}
	if metrics == nil {
		metrics = &Metrics{}
	}
	return &Server{store: store, ttl: uint32(ttl / time.Second), defaultNamespace: defaultNamespace, metrics: metrics}
}

func (s *Server) Resolve(ctx context.Context, name string) ([]net.IP, error) {
	s.metrics.IncDNSQuery()
	serviceName, ns, ok := parseName(name, s.defaultNamespace)
	if !ok {
		s.metrics.IncDNSError()
		return nil, fmt.Errorf("unsupported DNS name %q", name)
	}
	ctx = namespace.WithContext(ctx, ns)
	services, err := s.store.ListServices(ctx)
	if err != nil {
		s.metrics.IncDNSError()
		return nil, err
	}
	var service types.Service
	for _, v := range services {
		if v.Spec.Name == serviceName && v.Status == types.ServiceActive {
			service = v
			break
		}
	}
	if service.ID == "" {
		return nil, nil
	}
	tasks, err := s.store.ListTasks(ctx, controlplane.TaskFilter{ServiceID: service.ID})
	if err != nil {
		s.metrics.IncDNSError()
		return nil, err
	}
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		s.metrics.IncDNSError()
		return nil, err
	}
	addresses := map[types.NodeID]string{}
	for _, n := range nodes {
		addresses[n.ID] = n.AdvertiseAddress
	}
	out := []net.IP{}
	seen := map[string]struct{}{}
	for _, t := range tasks {
		if t.ActualStatus != types.TaskHealthy && t.ActualStatus != types.TaskRunning {
			continue
		}
		host, _, err := net.SplitHostPort(addresses[t.NodeID])
		if err != nil {
			host = addresses[t.NodeID]
		}
		ip := net.ParseIP(host).To4()
		if ip == nil {
			continue
		}
		if _, ok := seen[ip.String()]; !ok {
			seen[ip.String()] = struct{}{}
			out = append(out, ip)
		}
	}
	return out, nil
}
func parseName(name, defaultNS string) (string, string, bool) {
	parts := strings.Split(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "."), ".")
	if len(parts) == 4 && parts[1] == "svc" && parts[2] == "orch" {
		return "", "", false
	}
	if len(parts) == 3 && parts[1] == "svc" && parts[2] == "orch" {
		return parts[0], defaultNS, true
	}
	if len(parts) == 5 && parts[2] == "svc" && parts[3] == "orch" {
		return "", "", false
	}
	if len(parts) == 4 && parts[2] == "svc" && parts[3] == "orch" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func (s *Server) ServePacket(ctx context.Context, request []byte) ([]byte, error) {
	if len(request) < 12 {
		return nil, fmt.Errorf("short DNS packet")
	}
	qd := binary.BigEndian.Uint16(request[4:6])
	if qd != 1 {
		return nil, fmt.Errorf("exactly one question is required")
	}
	name, offset, err := decodeName(request, 12)
	if err != nil {
		return nil, err
	}
	if offset+4 > len(request) {
		return nil, fmt.Errorf("truncated DNS question")
	}
	qtype := binary.BigEndian.Uint16(request[offset : offset+2])
	questionEnd := offset + 4
	ips, err := s.Resolve(ctx, name)
	if err != nil {
		return nil, err
	}
	response := append([]byte(nil), request[:questionEnd]...)
	binary.BigEndian.PutUint16(response[2:4], 0x8180)
	binary.BigEndian.PutUint16(response[6:8], 0)
	if qtype != 1 {
		return response, nil
	}
	binary.BigEndian.PutUint16(response[6:8], uint16(len(ips)))
	for _, ip := range ips {
		v := ip.To4()
		if v == nil {
			continue
		}
		answer := make([]byte, 16)
		binary.BigEndian.PutUint16(answer[0:2], 0xc00c)
		binary.BigEndian.PutUint16(answer[2:4], 1)
		binary.BigEndian.PutUint16(answer[4:6], 1)
		binary.BigEndian.PutUint32(answer[6:10], s.ttl)
		binary.BigEndian.PutUint16(answer[10:12], 4)
		copy(answer[12:], v)
		response = append(response, answer...)
	}
	return response, nil
}
func decodeName(packet []byte, offset int) (string, int, error) {
	labels := []string{}
	for {
		if offset >= len(packet) {
			return "", 0, fmt.Errorf("truncated DNS name")
		}
		size := int(packet[offset])
		offset++
		if size == 0 {
			break
		}
		if size > 63 || offset+size > len(packet) {
			return "", 0, fmt.Errorf("invalid DNS label")
		}
		labels = append(labels, string(packet[offset:offset+size]))
		offset += size
	}
	return strings.Join(labels, "."), offset, nil
}

func (s *Server) ServeUDP(ctx context.Context, address string) error {
	conn, err := net.ListenPacket("udp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() { <-ctx.Done(); _ = conn.Close() }()
	buffer := make([]byte, 4096)
	for {
		n, peer, err := conn.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		response, err := s.ServePacket(ctx, buffer[:n])
		if err != nil {
			s.metrics.IncDNSError()
			continue
		}
		_, _ = conn.WriteTo(response, peer)
	}
}
