package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/oseayemenre/load_balancer/internal/config"
	"github.com/oseayemenre/load_balancer/internal/server"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

func run() error {
	sigctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	godotenv.Load()
	cfg := config.FromEnv()
	if err := cfg.Validate(); err != nil {
		return err
	}

	router := chi.NewRouter()
	servers, err := readServersFromYAMLConfig()
	if err != nil {
		return err
	}
	server.RegisterRoutes(router, servers, 0, &sync.Mutex{})

	svr := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: router,
	}

	errChan := make(chan error)

	fmt.Printf("server is starting on port %d\n", cfg.Port)
	go func() {
		if err := svr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-sigctx.Done():
		fmt.Printf("kill signal recieved\n")
		ctx, cancel := context.WithTimeout(sigctx, 15*time.Second)
		defer cancel()
		if err := svr.Shutdown(ctx); err != nil {
			return fmt.Errorf("error shutting down server, %v\n", err)
		}
	}

	return nil
}

type serversConfig struct {
	Servers []string `yaml:"servers"`
}

func readServersFromYAMLConfig() ([]string, error) {
	file, err := os.Open(filepath.Join("config.yml"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("yml config file not found")
		}
	}

	config := &serversConfig{}

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("error decoding config, %v", err)
	}

	return config.Servers, nil
}
