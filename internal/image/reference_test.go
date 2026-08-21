package image

import "testing"

func TestParseImageReferences(t *testing.T) {
	tests := []struct {
		reference string
		registry  string
		name      string
		tag       string
		digest    string
	}{
		{reference: "nginx:1.27", registry: "docker.io", name: "library/nginx", tag: "1.27"},
		{reference: "ghcr.io/acme/api:2.0", registry: "ghcr.io", name: "acme/api", tag: "2.0"},
		{reference: "localhost:5000/api", registry: "localhost:5000", name: "api", tag: "latest"},
		{reference: "ghcr.io/acme/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", registry: "ghcr.io", name: "acme/api", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	for _, tt := range tests {
		t.Run(tt.reference, func(t *testing.T) {
			got, err := Parse(tt.reference)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got.Registry != tt.registry || got.Name != tt.name || got.Tag != tt.tag || got.Digest != tt.digest {
				t.Fatalf("unexpected metadata: %#v", got)
			}
		})
	}
}

func TestPinnedReference(t *testing.T) {
	metadata, err := Parse("nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if got := PinnedReference(metadata); got != "docker.io/library/nginx@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("unexpected pinned reference %q", got)
	}
}
