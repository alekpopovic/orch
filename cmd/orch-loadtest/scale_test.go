//go:build scale

package main

import (
	"context"
	"testing"
	"time"
)

func TestThousandServiceScale(t *testing.T) {
	result, err := run(context.Background(), config{nodes: 50, services: 1000, replicas: 3, failureRate: 0.01, duration: time.Second, seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if result.TasksCreated < 3000 || result.ReconciliationIterations != 1000 {
		t.Fatalf("unexpected result %#v", result)
	}
}
