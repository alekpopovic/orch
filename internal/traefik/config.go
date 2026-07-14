package traefik

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/alekpopovic/orch/internal/discovery"
	"github.com/alekpopovic/orch/pkg/types"
)

type Config struct {
	HTTP HTTPConfig `json:"http"`
}

type HTTPConfig struct {
	Routers  map[string]Router  `json:"routers,omitempty"`
	Services map[string]Service `json:"services,omitempty"`
}

type Router struct {
	Rule    string     `json:"rule"`
	Service string     `json:"service"`
	TLS     *TLSConfig `json:"tls,omitempty"`
}

type TLSConfig struct{}

type Service struct {
	LoadBalancer LoadBalancer `json:"loadBalancer"`
}

type LoadBalancer struct {
	Servers        []Server `json:"servers"`
	PassHostHeader bool     `json:"passHostHeader"`
}

type Server struct {
	URL string `json:"url"`
}

func BuildConfig(services []types.Service, endpointSets []discovery.ServiceEndpoints) Config {
	config := Config{
		HTTP: HTTPConfig{
			Routers:  make(map[string]Router),
			Services: make(map[string]Service),
		},
	}
	endpointsByServiceID := endpointsByService(endpointSets)
	orderedServices := append([]types.Service(nil), services...)
	sortServices(orderedServices)

	for _, service := range orderedServices {
		if service.Status != "" && service.Status != types.ServiceActive {
			continue
		}
		endpointSet := endpointsByServiceID[service.ID]
		for routeIndex, route := range service.Spec.Routes {
			if err := route.Validate(); err != nil {
				continue
			}
			servers := routeServers(endpointSet.Endpoints, route)
			if len(servers) == 0 {
				continue
			}
			routerName := routeName(service, route, routeIndex)
			serviceName := routerName + "-service"
			router := Router{
				Rule:    routeRule(route),
				Service: serviceName,
			}
			if route.TLS {
				router.TLS = &TLSConfig{}
			}
			config.HTTP.Routers[routerName] = router
			config.HTTP.Services[serviceName] = Service{
				LoadBalancer: LoadBalancer{
					Servers:        servers,
					PassHostHeader: true,
				},
			}
		}
	}

	if len(config.HTTP.Routers) == 0 {
		config.HTTP.Routers = nil
	}
	if len(config.HTTP.Services) == 0 {
		config.HTTP.Services = nil
	}
	return config
}

func endpointsByService(endpointSets []discovery.ServiceEndpoints) map[types.ServiceID]discovery.ServiceEndpoints {
	endpointsByServiceID := make(map[types.ServiceID]discovery.ServiceEndpoints, len(endpointSets))
	for _, endpointSet := range endpointSets {
		endpointsByServiceID[endpointSet.ServiceID] = endpointSet
	}
	return endpointsByServiceID
}

func sortServices(services []types.Service) {
	sort.Slice(services, func(leftIndex, rightIndex int) bool {
		if services[leftIndex].Spec.Name == services[rightIndex].Spec.Name {
			return services[leftIndex].ID < services[rightIndex].ID
		}
		return services[leftIndex].Spec.Name < services[rightIndex].Spec.Name
	})
}

func routeServers(endpoints []discovery.Endpoint, route types.Route) []Server {
	servers := make([]Server, 0)
	for _, endpoint := range endpoints {
		if endpoint.ContainerPort != route.Port {
			continue
		}
		if endpoint.Protocol != types.PortTCP {
			continue
		}
		if !healthyEndpoint(endpoint.HealthStatus) {
			continue
		}
		serverURL, ok := endpointURL(endpoint)
		if !ok {
			continue
		}
		servers = append(servers, Server{URL: serverURL})
	}
	sort.Slice(servers, func(leftIndex, rightIndex int) bool {
		return servers[leftIndex].URL < servers[rightIndex].URL
	})
	return servers
}

func healthyEndpoint(status types.TaskStatus) bool {
	return status == types.TaskRunning || status == types.TaskHealthy
}

func endpointURL(endpoint discovery.Endpoint) (string, bool) {
	if endpoint.PublicHostPort <= 0 {
		return "", false
	}
	nodeAddress := strings.TrimSpace(endpoint.NodeAddress)
	if nodeAddress == "" {
		return "", false
	}
	host := addressHost(nodeAddress)
	if host == "" {
		return "", false
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(endpoint.PublicHostPort)), true
}

func addressHost(address string) string {
	parsed, err := url.Parse(address)
	if err == nil && parsed.Host != "" {
		return parsed.Hostname()
	}
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return strings.Trim(address, "[]")
}

func routeRule(route types.Route) string {
	return fmt.Sprintf("Host(%s) && PathPrefix(%s)", strconv.Quote(strings.TrimSpace(route.Host)), strconv.Quote(strings.TrimSpace(route.PathPrefix)))
}

func routeName(service types.Service, route types.Route, routeIndex int) string {
	return "orch-" + slug(fmt.Sprintf("%s-%s-%d-%d", service.Spec.Name, route.Host, route.Port, routeIndex))
}

func slug(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	slugged := strings.Trim(builder.String(), "-")
	if slugged == "" {
		return "route"
	}
	return slugged
}
