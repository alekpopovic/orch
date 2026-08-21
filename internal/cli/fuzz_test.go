package cli

import "testing"

func FuzzServiceYAML(f *testing.F) {
	for _, v := range [][]byte{[]byte("name: api\nimage: nginx:1.27\nreplicas: 1\n"), []byte("kind: nope\n"), nil} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v []byte) { _, _ = ParseDeploy(v) })
}
