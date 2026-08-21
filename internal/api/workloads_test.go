package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alekpopovic/orch/internal/controlplane"
	"github.com/alekpopovic/orch/pkg/types"
)

func requestJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestJobCronAndVolumeAPIs(t *testing.T) {
	service := controlplane.NewMemoryService()
	handler := NewHandler(nil, service)
	resources := types.ResourceRequirements{Requests: types.Resources{CPU: 100, Memory: 1 << 20}, Limits: types.Resources{CPU: 100, Memory: 1 << 20}}
	response := requestJSON(t, handler, http.MethodPost, "/v1/jobs", types.JobSpec{Name: "migrate", Image: "busybox:1.36", ResourceRequirements: resources})
	if response.Code != http.StatusCreated {
		t.Fatalf("job status=%d body=%s", response.Code, response.Body.String())
	}
	response = requestJSON(t, handler, http.MethodPost, "/v1/cronjobs", types.CronJobSpec{Name: "nightly", Schedule: "0 2 * * *", Timezone: "UTC", JobTemplate: types.JobSpec{Name: "nightly", Image: "busybox:1.36", ResourceRequirements: resources}})
	if response.Code != http.StatusCreated {
		t.Fatalf("cron status=%d body=%s", response.Code, response.Body.String())
	}
	response = requestJSON(t, handler, http.MethodPost, "/v1/volumes", types.Volume{Name: "data", Driver: "local"})
	if response.Code != http.StatusCreated {
		t.Fatalf("volume status=%d body=%s", response.Code, response.Body.String())
	}
	var volume VolumeResponse
	if err := json.Unmarshal(response.Body.Bytes(), &volume); err != nil {
		t.Fatal(err)
	}
	response = requestJSON(t, handler, http.MethodPost, "/v1/volume-claims", types.VolumeClaim{Name: "data", VolumeID: volume.Volume.ID, AccessMode: types.VolumeReadWriteOnce})
	if response.Code != http.StatusCreated {
		t.Fatalf("claim status=%d body=%s", response.Code, response.Body.String())
	}
	for _, path := range []string{"/v1/jobs", "/v1/cronjobs", "/v1/volumes", "/v1/volume-claims"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d", path, response.Code)
		}
	}
	response = requestJSON(t, handler, http.MethodPost, "/v1/notification-sinks", CreateNotificationSinkRequest{Name: "ops", Type: types.NotificationWebhook, URL: "https://hooks.example.test", SigningSecret: "never-return-this"})
	if response.Code != http.StatusCreated || bytes.Contains(response.Body.Bytes(), []byte("never-return-this")) {
		t.Fatalf("notification response status=%d body=%s", response.Code, response.Body.String())
	}
}
