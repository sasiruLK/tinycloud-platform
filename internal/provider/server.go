package provider

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sasiruLK/tinycloud-platform/internal/infra"
)

// Server hosts one in-tree Provider over the published contract.
//
// It is thin on purpose. Everything it does — bearer authentication, answering
// Capability discovery, refusing an undeclared Capability with 501, turning a
// Substrate failure into 502 — is what the contract asks of any Provider, so
// what runs here is the same contract a third-party Provider implements, not a
// privileged shortcut into Core.
type Server struct {
	provider Infra
	token    TokenSource
	caps     map[string]bool
	mux      *http.ServeMux
}

// NewServer returns a Server exposing p, authenticated with the token yielded
// by token. A nil token source leaves the Provider unauthenticated, which is
// acceptable only in tests: the contract requires a bearer token.
func NewServer(p Infra, token TokenSource) *Server {
	s := &Server{provider: p, token: token, caps: capabilitySet(p.Capabilities()), mux: http.NewServeMux()}

	// Anything the contract does not define answers in the contract's error
	// shape, so a Provider author debugging a typo reads JSON rather than
	// Go's default plain-text 404.
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, codeBadRequest, "no such path: "+r.URL.Path)
	})
	s.mux.HandleFunc(pathHealth, s.health)
	s.mux.HandleFunc(pathCapabilities, s.authenticated(s.capabilities))
	s.mux.HandleFunc(pathInstances, s.authenticated(s.capability(CapabilityInstances, s.instances)))
	s.mux.HandleFunc(pathMetrics, s.authenticated(s.capability(CapabilityMetrics, s.metrics)))
	s.mux.HandleFunc(pathAlarms, s.authenticated(s.capability(CapabilityAlarms, s.alarms)))
	s.mux.HandleFunc(pathIngress, s.authenticated(s.capability(CapabilityIngress, s.ingress)))
	s.mux.HandleFunc(pathBackups, s.authenticated(s.capability(CapabilityBackups, s.backups)))

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// health is the one unauthenticated path: a Kubernetes probe has no token, and
// saying "this process is up" reveals nothing about the Substrate.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if !allowGet(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// authenticated rejects a request whose bearer token is missing or wrong.
func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token == nil {
			next(w, r)
			return
		}
		want, err := s.token()
		if err != nil {
			// The Provider cannot tell whether the caller is authorised, so it
			// refuses everyone rather than falling open.
			log.Printf("[ERROR] provider token unavailable: %v", err)
			writeError(w, http.StatusUnauthorized, codeUnauthorized, "provider token unavailable")
			return
		}

		if subtle.ConstantTimeCompare([]byte(bearerToken(r)), []byte(want)) != 1 {
			writeError(w, http.StatusUnauthorized, codeUnauthorized, "missing or invalid bearer token")
			return
		}
		next(w, r)
	}
}

// bearerToken returns the token of an Authorization header, or the empty
// string when the header is missing or is not a bearer credential. The scheme
// is part of the header, so a bare token is not a token.
func bearerToken(r *http.Request) string {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// capability refuses a Capability this Provider has not declared, before the
// implementation is reached. Discovery and behaviour therefore cannot disagree:
// what a Provider does not list, it does not serve.
func (s *Server) capability(name string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.caps[name] {
			writeError(w, http.StatusNotImplemented, codeNotImplemented,
				"capability "+name+" is not implemented by provider "+s.provider.Name())
			return
		}
		next(w, r)
	}
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	if !allowGet(w, r) {
		return
	}
	declared := []string{}
	for _, c := range InfraCapabilities {
		if s.caps[c] {
			declared = append(declared, c)
		}
	}
	writeJSON(w, http.StatusOK, capabilitiesBody{
		Kind:         KindInfra,
		Provider:     s.provider.Name(),
		Capabilities: declared,
	})
}

func (s *Server) instances(w http.ResponseWriter, r *http.Request) {
	if !allowGet(w, r) {
		return
	}
	list, err := s.provider.Instances(r.Context())
	if err != nil {
		s.writeFailure(w, CapabilityInstances, err)
		return
	}
	if list == nil {
		list = []infra.InstanceInfo{}
	}
	writeJSON(w, http.StatusOK, instancesBody{Instances: list})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if !allowGet(w, r) {
		return
	}
	metric := r.URL.Query().Get("metric")
	if !ValidMetric(metric) {
		writeError(w, http.StatusBadRequest, codeBadRequest,
			"unknown metric "+strconv.Quote(metric)+"; the contract defines "+strings.Join(infra.Metrics, ", "))
		return
	}

	seconds, err := strconv.Atoi(r.URL.Query().Get("window"))
	if err != nil || seconds <= 0 {
		writeError(w, http.StatusBadRequest, codeBadRequest, "window must be a positive number of seconds")
		return
	}

	series, err := s.provider.Metric(r.Context(), metric, time.Duration(seconds)*time.Second)
	if err != nil {
		s.writeFailure(w, CapabilityMetrics, err)
		return
	}
	if series == nil {
		series = []infra.Series{}
	}
	writeJSON(w, http.StatusOK, seriesBody{Series: series})
}

func (s *Server) alarms(w http.ResponseWriter, r *http.Request) {
	if !allowGet(w, r) {
		return
	}
	list, err := s.provider.Alarms(r.Context())
	if err != nil {
		s.writeFailure(w, CapabilityAlarms, err)
		return
	}
	if list == nil {
		list = []infra.AlarmStatus{}
	}
	writeJSON(w, http.StatusOK, alarmsBody{Alarms: list})
}

func (s *Server) ingress(w http.ResponseWriter, r *http.Request) {
	if !allowGet(w, r) {
		return
	}
	address, err := s.provider.IngressAddress(r.Context())
	if err != nil {
		s.writeFailure(w, CapabilityIngress, err)
		return
	}
	writeJSON(w, http.StatusOK, ingressBody{PublicIP: address})
}

func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	if !allowGet(w, r) {
		return
	}
	listing, err := s.provider.BackupObjects(r.Context())
	if err != nil {
		s.writeFailure(w, CapabilityBackups, err)
		return
	}
	objects := listing.Objects
	if objects == nil {
		objects = []infra.ObjectInfo{}
	}
	writeJSON(w, http.StatusOK, backupsBody{Store: listing.Store, Objects: objects})
}

// writeFailure maps an implementation's error onto the contract: a Capability
// the Provider turns out not to serve is 501, anything else is the Substrate
// failing underneath it and is 502. Neither is Core's fault, and both reach the
// dashboard as a named warning rather than a blank page.
func (s *Server) writeFailure(w http.ResponseWriter, capability string, err error) {
	if errors.Is(err, ErrNotImplemented) {
		writeError(w, http.StatusNotImplemented, codeNotImplemented,
			"capability "+capability+" is not implemented by provider "+s.provider.Name())
		return
	}
	log.Printf("[WARN] provider %s: %s: %v", s.provider.Name(), capability, err)
	writeError(w, http.StatusBadGateway, codeUpstreamError, capability+": "+err.Error())
}

// allowGet reports whether the request may proceed, answering anything but GET
// itself. Every Capability of the Infra kind is a read.
func allowGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeError(w, http.StatusMethodNotAllowed, codeBadRequest, "only GET is supported")
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("[WARN] provider response encode failed: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: code, Message: message})
}
