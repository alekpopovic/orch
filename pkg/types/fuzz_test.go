package types

import "testing"

func FuzzResourceQuantities(f *testing.F) {
	for _, v := range []string{"500m", "1", "128Mi", "0", "-1", "garbage"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) { _, _ = ParseCPU(v); _, _ = ParseMemory(v) })
}
func FuzzPortSpec(f *testing.F) {
	for _, v := range []string{"8080:80/tcp", "53/udp", "0", "65536", "abc"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) { _, _ = ParsePortSpec(v) })
}
func FuzzLabelSelector(f *testing.F) {
	for _, v := range []string{"region=eu", "disk!=slow", "a=b,c!=d", "bad"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) { _, _ = ParseLabelSelector(v) })
}
func FuzzHealthcheckPath(f *testing.F) {
	for _, v := range []string{"/healthz", "//evil", "http://evil", "/ok?q=1", "\x00"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) { _ = (&Healthcheck{Type: HealthcheckHTTP, Path: v, Port: 80}).Validate() })
}
