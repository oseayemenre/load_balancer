package server

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

var (
	servers = []string{"localhost:3001", "localhost:3002", "localhost:3003"}
	index   = 0
	mu      = sync.Mutex{}
)

func RegisterRoutes(r *chi.Mux) {
	r.HandleFunc("*", func(w http.ResponseWriter, r *http.Request) {
		var req *http.Request
		var err error

		mu.Lock()
		current := index
		index = (index + 1) % len(servers)
		target := servers[current]
		mu.Unlock()

		if r.Body != nil {
			req, err = http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		} else {
			req, err = http.NewRequestWithContext(r.Context(), r.Method, target, nil)
		}

		req.Header = r.Header.Clone()

		if err != nil {
			http.Error(w, fmt.Sprintf("error building request, %v", err), http.StatusInternalServerError)
			return
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, fmt.Sprintf("error sending request, %v", err), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		parsed, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("error reading response body, %v", err), http.StatusInternalServerError)
			return
		}

		writeJSON(w, resp.StatusCode, parsed)
	})
}

func writeJSON(w http.ResponseWriter, code int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}
