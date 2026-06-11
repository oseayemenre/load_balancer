package server

import (
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
)

type LbConfig struct {
	Servers []LbServersConfig `yaml:"servers"`
}

type LbServersConfig struct {
	Server      string `yaml:"server"`
	Weight      int    `yaml:"weight"`
	Connections int
}

func RegisterRoutes(
	r *chi.Mux,
	servers []LbServersConfig,
	algo string) {
	var (
		current     int
		weightCount = 1
		mu          sync.Mutex
	)

	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		var req *http.Request
		var err error
		var target string

		switch algo {
		case "ip hash":
			target, err = ipHash(r.RemoteAddr, servers)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

		case "least connection":
			target = leastConnection(&mu, servers, current)
			defer func() {
				mu.Lock()
				servers[current].Connections--
				mu.Unlock()
			}()
		default:
			current, weightCount, target = roundrobin(&mu, servers, current, weightCount)
		}

		url := fmt.Sprintf("%s%s", target, r.RequestURI)
		switch r.Method {
		case http.MethodGet, http.MethodDelete, http.MethodOptions:
			req, err = http.NewRequestWithContext(r.Context(), r.Method, url, nil)
		default:
			req, err = http.NewRequestWithContext(r.Context(), r.Method, url, r.Body)
		}

		if err != nil {
			http.Error(w, fmt.Sprintf("error building request, %v", err), http.StatusInternalServerError)
			return
		}
		req.Header = r.Header.Clone()
		req.Header.Set("X-Forwarded-For", r.RemoteAddr)

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

		writeResponse(w, resp.StatusCode, resp.Header, parsed)
	})
}

func roundrobin(mu *sync.Mutex, servers []LbServersConfig, current int, weightCount int) (int, int, string) {
	mu.Lock()
	target := servers[current].Server
	if weightCount == servers[current].Weight {
		weightCount = 1
		current = (current + 1) % len(servers)
	} else {
		weightCount++
	}
	mu.Unlock()
	return current, weightCount, target
}

func ipHash(remoteAddr string, servers []LbServersConfig) (string, error) {
	h := fnv.New32a()
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return "", fmt.Errorf("error splitting host port, %v", err)
	}
	h.Write([]byte(ip))
	current := int(h.Sum32()) % len(servers)
	return servers[current].Server, nil
}

func leastConnection(mu *sync.Mutex, servers []LbServersConfig, current int) string {
	mu.Lock()
	for i := range servers {
		if servers[i].Connections/servers[i].Weight < servers[current].Connections/servers[current].Weight {
			current = i
		}
	}
	servers[current].Connections++
	mu.Unlock()
	return servers[current].Server

}

func writeResponse(w http.ResponseWriter, code int, header http.Header, data []byte) {
	for k, v := range header {
		w.Header()[k] = v
	}
	w.WriteHeader(code)
	w.Write(data)
}
