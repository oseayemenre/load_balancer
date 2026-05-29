package server

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

type LbConfig struct {
	Servers []LbServersConfig `yaml:"servers"`
}

type LbServersConfig struct {
	Server string `yaml:"server"`
	Weight int    `yaml:"weight"`
}

func RegisterRoutes(r *chi.Mux, servers []LbServersConfig, index, weightCount int, mu *sync.Mutex) {
	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		var req *http.Request
		var err error

		mu.Lock()
		current := index
		if servers[current].Weight == 0 {
			servers[current].Weight = 1
		}
		target := fmt.Sprintf("%s%s", servers[current].Server, r.RequestURI)
		if weightCount == servers[current].Weight {
			weightCount = 1
			index = (index + 1) % len(servers)
		} else {
			weightCount++
		}
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

		writeResponse(w, resp.StatusCode, parsed)
	})
}

func writeResponse(w http.ResponseWriter, code int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}
