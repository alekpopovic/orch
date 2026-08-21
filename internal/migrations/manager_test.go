package migrations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeLocker struct {
	locked, unlocked int
	err              error
}

func TestDiscoverStatusInputIsIdempotent(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"000002_second.up.sql", "000001_first.up.sql", "000001_first.down.sql"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	first, err := Discover(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Discover(directory)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || len(first) != 2 || first[0].Version != "000001" {
		t.Fatalf("non-deterministic migration discovery: %#v / %#v", first, second)
	}
}

func (f *fakeLocker) Lock(context.Context) error   { f.locked++; return f.err }
func (f *fakeLocker) Unlock(context.Context) error { f.unlocked++; return nil }
func TestMigrationLock(t *testing.T) {
	f := &fakeLocker{}
	called := false
	if err := WithLock(context.Background(), f, func() error { called = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !called || f.locked != 1 || f.unlocked != 1 {
		t.Fatalf("lock lifecycle %#v", f)
	}
}
func TestMigrationLockFailure(t *testing.T) {
	f := &fakeLocker{err: errors.New("busy")}
	if WithLock(context.Background(), f, func() error { return nil }) == nil {
		t.Fatal("expected lock failure")
	}
	if f.unlocked != 0 {
		t.Fatal("unlocked without lock")
	}
}
