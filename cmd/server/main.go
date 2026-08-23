package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BABTUNA/queuemaxxing/internal/httpapi"
	"github.com/BABTUNA/queuemaxxing/internal/service"
	"github.com/BABTUNA/queuemaxxing/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() (returnErr error) {
	wal, err := storage.Open(environment("WAL_PATH", "./data/queue.wal"))
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, wal.Close())
	}()

	manager, err := service.NewManager(wal, time.Now())
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + environment("PORT", "8080"),
		Handler:           httpapi.NewHandler(manager),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverError := make(chan error, 1)
	go func() {
		log.Printf("queue server listening on %s", server.Addr)
		serverError <- server.ListenAndServe()
	}()

	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}

func environment(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
