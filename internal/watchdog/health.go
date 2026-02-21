package watchdog

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status  string      `json:"status"`
	Targets []StateInfo `json:"targets,omitempty"`
}

func HealthzHandler(w *Watchdog) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(healthResponse{
			Status: "ok",
		})
	}
}

func ReadyzHandler(w *Watchdog) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")

		states := w.GetAllStates()
		resp := healthResponse{
			Targets: states,
		}

		if w.IsReady() {
			resp.Status = "ready"
			rw.WriteHeader(http.StatusOK)
		} else {
			resp.Status = "not_ready"
			rw.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(rw).Encode(resp)
	}
}
