package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/alekpopovic/orch/pkg/types"
)

func TestQuotaAPIProducesStructuredConflict(t *testing.T) {
	handler := newTestHandler()
	set := doRequest(t, handler, http.MethodPut, "/v1/quota", `{"max_services":1}`)
	if set.Code != http.StatusOK {
		t.Fatalf("set quota: %d %s", set.Code, set.Body.String())
	}
	for i, name := range []string{"first", "second"} {
		response := doRequest(t, handler, http.MethodPost, "/v1/services", `{"spec":{"name":"`+name+`","image":"nginx:1.27","replicas":0}}`)
		if i == 0 && response.Code != http.StatusCreated {
			t.Fatalf("first service: %d %s", response.Code, response.Body.String())
		}
		if i == 1 {
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"quota_exceeded"`) || !strings.Contains(response.Body.String(), `"resource":"services"`) {
				t.Fatalf("expected structured quota conflict, got %d %s", response.Code, response.Body.String())
			}
		}
	}
}

type fakeGitOpsSyncer struct {
	source types.GitOpsSource
}

func (syncer *fakeGitOpsSyncer) Sync(_ context.Context, id string) (types.GitOpsSource, error) {
	syncer.source.ID = id
	syncer.source.LastRevision = "abc123"
	return syncer.source, nil
}

func TestGitOpsSourceCRUDAndManualSyncAPI(t *testing.T) {
	syncer := &fakeGitOpsSyncer{}
	handler := newTestHandler(WithGitOpsSyncer(syncer))
	created := doRequest(t, handler, http.MethodPost, "/v1/gitops/sources", `{"repository_url":"https://example.com/acme/config.git","branch":"main","path":"services","sync_interval":"1m","prune":true}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create source: %d %s", created.Code, created.Body.String())
	}
	var response GitOpsSourceResponse
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Source.ID == "" {
		t.Fatal("expected source id")
	}
	listed := doRequest(t, handler, http.MethodGet, "/v1/gitops/sources", nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), response.Source.ID) {
		t.Fatalf("list sources: %d %s", listed.Code, listed.Body.String())
	}
	synced := doRequest(t, handler, http.MethodPost, "/v1/gitops/sources/"+response.Source.ID+"/sync", nil)
	if synced.Code != http.StatusOK || !strings.Contains(synced.Body.String(), "abc123") {
		t.Fatalf("sync source: %d %s", synced.Code, synced.Body.String())
	}
	deleted := doRequest(t, handler, http.MethodDelete, "/v1/gitops/sources/"+response.Source.ID, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete source: %d %s", deleted.Code, deleted.Body.String())
	}
}
