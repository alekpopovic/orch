package api

import (
	"net/http"
	"strings"

	"github.com/alekpopovic/orch/internal/notifications"
	"github.com/alekpopovic/orch/pkg/types"
)

type JobResponse struct {
	Job types.Job `json:"job"`
}
type JobsResponse struct {
	Jobs []types.Job `json:"jobs"`
}
type CronJobResponse struct {
	CronJob types.CronJob `json:"cronjob"`
}
type CronJobsResponse struct {
	CronJobs []types.CronJob `json:"cronjobs"`
}
type VolumeResponse struct {
	Volume types.Volume `json:"volume"`
}
type VolumesResponse struct {
	Volumes []types.Volume `json:"volumes"`
}
type VolumeClaimResponse struct {
	Claim types.VolumeClaim `json:"claim"`
}
type VolumeClaimsResponse struct {
	Claims []types.VolumeClaim `json:"claims"`
}
type NotificationSinkResponse struct {
	Sink types.NotificationSink `json:"sink"`
}
type CreateNotificationSinkRequest struct {
	Name          string                     `json:"name"`
	Type          types.NotificationSinkType `json:"type"`
	URL           string                     `json:"url"`
	SigningSecret string                     `json:"signing_secret,omitempty"`
}
type NotificationSinksResponse struct {
	Sinks []types.NotificationSink `json:"sinks"`
}

func (s *Server) gitopsStatus(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.GitOpsStatus(r.Context())
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, ListServicesResponse{Services: v})
}
func (s *Server) gitopsDiff(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.GitOpsDiff(r.Context(), strings.TrimSpace(r.PathValue("service")))
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var v types.JobSpec
	if !s.decodeJSON(w, r, &v) {
		return
	}
	out, e := s.controlPlane.CreateJob(r.Context(), v)
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusCreated, JobResponse{out})
}
func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.ListJobs(r.Context())
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, JobsResponse{v})
}
func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.GetJob(r.Context(), r.PathValue("id"))
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, JobResponse{v})
}
func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	if e := s.controlPlane.DeleteJob(r.Context(), r.PathValue("id")); e != nil {
		s.writeError(w, r, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) createCronJob(w http.ResponseWriter, r *http.Request) {
	var v types.CronJobSpec
	if !s.decodeJSON(w, r, &v) {
		return
	}
	out, e := s.controlPlane.CreateCronJob(r.Context(), v)
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusCreated, CronJobResponse{out})
}
func (s *Server) listCronJobs(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.ListCronJobs(r.Context())
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, CronJobsResponse{v})
}
func (s *Server) getCronJob(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.GetCronJob(r.Context(), r.PathValue("id"))
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, CronJobResponse{v})
}
func (s *Server) deleteCronJob(w http.ResponseWriter, r *http.Request) {
	if e := s.controlPlane.DeleteCronJob(r.Context(), r.PathValue("id")); e != nil {
		s.writeError(w, r, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) suspendCronJob(w http.ResponseWriter, r *http.Request) {
	s.setCronSuspended(w, r, true)
}
func (s *Server) resumeCronJob(w http.ResponseWriter, r *http.Request) {
	s.setCronSuspended(w, r, false)
}
func (s *Server) setCronSuspended(w http.ResponseWriter, r *http.Request, value bool) {
	v, e := s.controlPlane.SetCronJobSuspended(r.Context(), r.PathValue("id"), value)
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, CronJobResponse{v})
}
func (s *Server) createVolume(w http.ResponseWriter, r *http.Request) {
	var v types.Volume
	if !s.decodeJSON(w, r, &v) {
		return
	}
	out, e := s.controlPlane.CreateVolume(r.Context(), v)
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusCreated, VolumeResponse{out})
}
func (s *Server) listVolumes(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.ListVolumes(r.Context())
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, VolumesResponse{v})
}
func (s *Server) getVolume(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.GetVolume(r.Context(), r.PathValue("id"))
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, VolumeResponse{v})
}
func (s *Server) deleteVolume(w http.ResponseWriter, r *http.Request) {
	if e := s.controlPlane.DeleteVolume(r.Context(), r.PathValue("id")); e != nil {
		s.writeError(w, r, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) createVolumeClaim(w http.ResponseWriter, r *http.Request) {
	var v types.VolumeClaim
	if !s.decodeJSON(w, r, &v) {
		return
	}
	out, e := s.controlPlane.CreateVolumeClaim(r.Context(), v)
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusCreated, VolumeClaimResponse{out})
}
func (s *Server) listVolumeClaims(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.ListVolumeClaims(r.Context())
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, VolumeClaimsResponse{v})
}
func (s *Server) createNotificationSink(w http.ResponseWriter, r *http.Request) {
	var request CreateNotificationSinkRequest
	if !s.decodeJSON(w, r, &request) {
		return
	}
	v := types.NotificationSink{Name: request.Name, Type: request.Type, URL: request.URL, SigningSecret: request.SigningSecret}
	out, e := s.controlPlane.CreateNotificationSink(r.Context(), v)
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusCreated, NotificationSinkResponse{out})
}
func (s *Server) listNotificationSinks(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.ListNotificationSinks(r.Context())
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, NotificationSinksResponse{v})
}
func (s *Server) deleteNotificationSink(w http.ResponseWriter, r *http.Request) {
	if e := s.controlPlane.DeleteNotificationSink(r.Context(), r.PathValue("id")); e != nil {
		s.writeError(w, r, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) testNotificationSink(w http.ResponseWriter, r *http.Request) {
	v, e := s.controlPlane.GetNotificationSink(r.Context(), r.PathValue("id"))
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	e = notifications.New().Deliver(r.Context(), v, types.Event{Namespace: v.Namespace, Type: "notification.test", Severity: types.EventInfo, Source: "api", Message: "notification sink test", Timestamp: s.now()})
	if e != nil {
		s.writeError(w, r, e)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "delivered"})
}
