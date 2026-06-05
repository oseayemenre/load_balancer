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
		index       int
		weightCount = 1
		mu          sync.Mutex
	)

	r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
		var req *http.Request
		var err error
		var target string
		var current int

		switch algo {
		case "ip hash":
			h := fnv.New32a()
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				http.Error(w, fmt.Sprintf("error splitting host port, %v", err), http.StatusBadRequest)
				return
			}
			h.Write([]byte(ip))
			current = int(h.Sum32()) % len(servers)
			target = fmt.Sprintf("%s%s", servers[current].Server, r.RequestURI)

		case "least connection":
			mu.Lock()
			for i := range servers {
				if servers[i].Connections < servers[current].Connections {
					current = i
				}
			}
			servers[current].Connections++
			target = fmt.Sprintf("%s%s", servers[current].Server, r.RequestURI)
			mu.Unlock()

			defer func() {
				mu.Lock()
				servers[current].Connections--
				mu.Unlock()
			}()
		default:
			mu.Lock()
			current = index
			target = fmt.Sprintf("%s%s", servers[current].Server, r.RequestURI)
			if weightCount == servers[current].Weight {
				weightCount = 1
				index = (index + 1) % len(servers)
			} else {
				weightCount++
			}
			mu.Unlock()
		}

		if r.Body != nil {
			req, err = http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		} else {
			req, err = http.NewRequestWithContext(r.Context(), r.Method, target, nil)
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

func writeResponse(w http.ResponseWriter, code int, header http.Header, data []byte) {
	for k, v := range header {
		w.Header()[k] = v
	}
	w.WriteHeader(code)
	w.Write(data)
}
