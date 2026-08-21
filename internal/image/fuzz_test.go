package image

import "testing"

func FuzzImageReference(f *testing.F) {
	for _, v := range []string{"nginx:1.27", "ghcr.io/acme/api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", "-bad", "x@@y"} {
		f.Add(v)
	}
	f.Fuzz(func(_ *testing.T, v string) {
		metadata, err := Parse(v)
		if err == nil {
			_ = PinnedReference(metadata)
		}
	})
}
