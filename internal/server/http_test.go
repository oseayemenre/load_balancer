package server

import (
	"hash/fnv"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRoundRobin(t *testing.T) {
	mu := &sync.Mutex{}
	servers := []LbServersConfig{
		{
			Server: "http://localhost:3000",
			Weight: 1,
		},
		{
			Server: "http://localhost:3001",
			Weight: 2,
		},
		{
			Server: "http://localhost:3002",
			Weight: 3,
		},
	}
	current := 0
	weightCount := 1
	target := ""

	expect := []string{
		"http://localhost:3000",
		"http://localhost:3001",
		"http://localhost:3001",
		"http://localhost:3002",
		"http://localhost:3002",
		"http://localhost:3002",
	}

	got := []string{}

	for range expect {
		current, weightCount, target = roundrobin(mu, servers, current, weightCount)
		got = append(got, target)
	}

	for i := range expect {
		if expect[i] != got[i] {
			t.Fatalf("expected %s, got %s", expect[i], got[i])
		}
	}
}

func TestIPHash(t *testing.T) {
	servers := []LbServersConfig{
		{Server: "http://localhost:3000"},
		{Server: "http://localhost:3001"},
		{Server: "http://localhost:3002"},
	}

	forwardedHeaders := []string{
		"10.0.0.1:2134",
		"10.0.0.2:5832",
		"10.0.0.3:8585",
		"10.0.0.1:4839",
		"10.0.0.2:2917",
		"10.0.0.3:2910",
	}

	expect := []string{
		servers[hash(t, forwardedHeaders[0])].Server,
		servers[hash(t, forwardedHeaders[1])].Server,
		servers[hash(t, forwardedHeaders[2])].Server,
		servers[hash(t, forwardedHeaders[3])].Server,
		servers[hash(t, forwardedHeaders[4])].Server,
		servers[hash(t, forwardedHeaders[5])].Server,
	}

	got := []string{}

	target := ""

	for _, h := range forwardedHeaders {
		target, _ = ipHash(h, servers)
		got = append(got, target)
	}

	for i := range expect {
		if expect[i] != got[i] {
			t.Fatalf("expected %s, got %s", expect[i], got[i])
		}
	}
}

func TestLeastConnection(t *testing.T) {
	mu := &sync.Mutex{}
	servers := []LbServersConfig{
		{
			Server:      "http://localhost:3000",
			Weight:      5,
			Connections: 7,
		},
		{
			Server:      "http://localhost:3001",
			Weight:      7,
			Connections: 9,
		},
		{
			Server:      "http://localhost:3002",
			Weight:      13,
			Connections: 12,
		},
	}

	current := 0
	target := leastConnection(mu, servers, current)
	if target != servers[2].Server {
		t.Fatalf("expected %s, got %s", servers[2].Server, target)
	}
}

func TestWriteResponse(t *testing.T) {
	w := httptest.NewRecorder()
	ip := "10.0.0.1:3291"
	writeResponse(w, http.StatusOK, http.Header{"X-Forwarded-For": []string{ip}}, []byte("test"))
	if val := w.Header().Get("X-Forwarded-For"); val != ip {
		t.Fatalf("expected %s, got %s", ip, val)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "test" {
		t.Fatalf("expected test got %s", w.Body.String())
	}
}

func TestProxyRequestRoundRobin(t *testing.T) {
	svr1 := setFakeServer(t, "server 1")
	svr2 := setFakeServer(t, "server 2")

	servers := []LbServersConfig{
		{
			Server: svr1.URL,
			Weight: 2,
		},
		{
			Server: svr2.URL,
			Weight: 1,
		},
	}

	r := chi.NewRouter()
	RegisterRoutes(r, servers, "round robin")
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req)
	if w1.Body.String() != "server 1" {
		t.Fatalf("expected server 1, got %v", w1.Body.String())
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	if w2.Body.String() != "server 1" {
		t.Fatalf("expected server 1, got %v", w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req)
	if w3.Body.String() != "server 2" {
		t.Fatalf("expected server 2, got %v", w3.Body.String())
	}
}

func TestProxyRequestIPHash(t *testing.T) {
	svr1 := setFakeServer(t, "server 1")
	svr2 := setFakeServer(t, "server 2")

	servers := []LbServersConfig{
		{
			Server: svr1.URL,
		},
		{
			Server: svr2.URL,
		},
	}

	r := chi.NewRouter()
	RegisterRoutes(r, servers, "ip hash")
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.1:2134"
	w1 := httptest.NewRecorder()
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w1, req)
	r.ServeHTTP(w2, req)

	if w2.Body.String() != w1.Body.String() {
		t.Fatal("expected requests to go to the same backend")
	}
}

func setFakeServer(t *testing.T, message string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected http method to be %s, got %s", http.MethodGet, r.Method)
		}
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Fatalf("expected X-Forwarded-For header to be set")
		}
		w.Write([]byte(message))
	}))
}

func hash(t *testing.T, server string) int {
	t.Helper()
	h := fnv.New32a()
	ip, _, err := net.SplitHostPort(server)
	if err != nil {
		t.Fatalf("error splitting host port, %v", err)
	}
	h.Write([]byte(ip))
	return int(h.Sum32()) % 3
}
