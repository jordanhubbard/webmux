package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jordanhubbard/webmux/server/internal/auth"
	"github.com/jordanhubbard/webmux/server/internal/usbforward"
)

func (s *Server) registerUSBForwardings(mux *http.ServeMux) {
	mux.Handle("GET /api/usb-forwardings", s.protected(false, s.listUSBForwardings))
	mux.Handle("POST /api/usb-forwardings", newLimiter(60, time.Minute).wrap(s.protected(false, s.createUSBForwarding)))
	mux.Handle("POST /api/usb-forwardings/{id}/stop", s.protected(false, s.stopUSBForwarding))
	mux.Handle("POST /api/usb-forwardings/{id}/claim", newLimiter(64, time.Minute).wrap(http.HandlerFunc(s.claimUSBForwarding)))
	mux.Handle("POST /api/usb-forwardings/{id}/report", newLimiter(4096, time.Minute).wrap(http.HandlerFunc(s.reportUSBForwarding)))
}
func (s *Server) usbLeaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usbforward.ErrLeaseInvalid):
		writeError(w, 400, err.Error())
	case errors.Is(err, usbforward.ErrLeaseNotFound):
		writeError(w, 404, err.Error())
	case errors.Is(err, usbforward.ErrLeaseConflict):
		writeError(w, 409, err.Error())
	case errors.Is(err, usbforward.ErrLeaseLimit):
		writeError(w, 429, err.Error())
	default:
		s.internalError(w, err)
	}
}
func (s *Server) listUSBForwardings(w http.ResponseWriter, r *http.Request, owner string) {
	w.Header().Set("Cache-Control", "no-store")
	leases, err := s.usbLeases.List(owner)
	if err != nil {
		s.usbLeaseError(w, err)
		return
	}
	writeJSON(w, 200, leases)
}
func (s *Server) createUSBForwarding(w http.ResponseWriter, r *http.Request, owner string) {
	var spec usbforward.LeaseSpec
	if !decodeJSONLimit(w, r, &spec, 4096) {
		return
	}
	ticket, err := s.usbLeases.Create(owner, spec)
	if err != nil {
		s.usbLeaseError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 201, ticket)
}
func (s *Server) stopUSBForwarding(w http.ResponseWriter, r *http.Request, owner string) {
	value, err := s.usbLeases.Stop(owner, r.PathValue("id"))
	if err != nil {
		s.usbLeaseError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, value)
}
func (s *Server) usbReporterOwners(w http.ResponseWriter) (func(string) bool, bool) {
	config, err := auth.LoadConfig(s.store)
	if err != nil {
		s.internalError(w, err)
		return nil, false
	}
	return func(owner string) bool {
		return config.Auth.Mode == "local" && config.HasUser(owner) || config.Auth.Mode == "none" && owner == "anonymous"
	}, true
}
func (s *Server) claimUSBForwarding(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string               `json:"pair_code"`
		Spec usbforward.LeaseSpec `json:"spec"`
	}
	if !decodeJSONLimit(w, r, &body, 4096) {
		return
	}
	allowed, ok := s.usbReporterOwners(w)
	if !ok {
		return
	}
	value, err := s.usbLeases.Claim(r.PathValue("id"), body.Code, body.Spec, allowed)
	if err != nil {
		s.usbLeaseError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, value)
}
func (s *Server) reportUSBForwarding(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeError(w, 404, "USB forwarding lease unavailable")
		return
	}
	var report usbforward.LeaseReport
	if !decodeJSONLimit(w, r, &report, 4096) {
		return
	}
	allowed, ok := s.usbReporterOwners(w)
	if !ok {
		return
	}
	value, err := s.usbLeases.Report(r.PathValue("id"), strings.TrimPrefix(header, "Bearer "), report, allowed)
	if err != nil {
		s.usbLeaseError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, value)
}
