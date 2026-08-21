package api

import (
	"fmt"
	"github.com/alekpopovic/orch/internal/maintenance"
	"github.com/alekpopovic/orch/internal/store"
	"github.com/alekpopovic/orch/pkg/types"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type MaintenanceWindowResponse struct {
	Window types.MaintenanceWindow `json:"window"`
}
type MaintenanceWindowsResponse struct {
	Windows []types.MaintenanceWindow `json:"windows"`
}

func withForceRequest(r *http.Request) *http.Request {
	value, _ := strconv.ParseBool(r.URL.Query().Get("force"))
	if value {
		return r.WithContext(maintenance.WithForce(r.Context()))
	}
	return r
}
func (s *Server) createMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	var v types.MaintenanceWindow
	if !s.decodeJSON(w, r, &v) {
		return
	}
	out, err := s.controlPlane.CreateMaintenanceWindow(r.Context(), v)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, MaintenanceWindowResponse{out})
}
func (s *Server) listMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	out, err := s.controlPlane.ListMaintenanceWindows(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MaintenanceWindowsResponse{out})
}
func (s *Server) deleteMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	if err := s.controlPlane.DeleteMaintenanceWindow(r.Context(), r.PathValue("id")); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) retentionStatus(w http.ResponseWriter, r *http.Request) {
	v, err := s.controlPlane.GetRetentionStatus(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) pruneRetention(w http.ResponseWriter, r *http.Request) {
	dry, _ := strconv.ParseBool(r.URL.Query().Get("dry_run"))
	v, err := s.controlPlane.PruneRetention(r.Context(), dry)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) getUsage(w http.ResponseWriter, r *http.Request) {
	from, err := parseReportTime(r.URL.Query().Get("from"))
	if err != nil {
		s.writeError(w, r, fmt.Errorf("%w: invalid from", store.ErrInvalidState))
		return
	}
	to, err := parseReportTime(r.URL.Query().Get("to"))
	if err != nil {
		s.writeError(w, r, fmt.Errorf("%w: invalid to", store.ErrInvalidState))
		return
	}
	v, err := s.controlPlane.GetUsageReport(r.Context(), strings.TrimSpace(r.URL.Query().Get("namespace")), from, to)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func parseReportTime(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, v); err == nil {
		return parsed.UTC(), nil
	}
	return time.Parse("2006-01-02", v)
}
