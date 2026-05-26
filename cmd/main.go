package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/oseayemenre/load_balancer/internal/config"
	"github.com/oseayemenre/load_balancer/internal/server"
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
	server.RegisterRoutes(router)

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
