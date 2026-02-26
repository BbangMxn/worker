package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"worker_server/config"
	"worker_server/internal/bootstrap"
	"worker_server/pkg/logger"

	"github.com/joho/godotenv"
)

const (
	shutdownTimeout = 30 * time.Second // Maximum time to wait for graceful shutdown
)

func main() {
	// Initialize logger early
	logger.Init(logger.Config{
		Level:   logger.LevelInfo,
		Service: "bridgify",
	})

	// Load .env file if exists (for local development)
	if err := godotenv.Load(); err != nil {
		logger.Debug("No .env file found, using environment variables")
	}

	mode := flag.String("mode", "all", "Run mode: api, worker, all")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config: %v", err)
	}

	switch *mode {
	case "api":
		runAPI(cfg)
	case "worker":
		runWorker(cfg)
	case "all":
		go runWorker(cfg)
		runAPI(cfg)
	default:
		logger.Fatal("Unknown mode: %s", *mode)
	}
}

func runAPI(cfg *config.Config) {
	app, cleanup, err := bootstrap.NewAPI(cfg)
	if err != nil {
		logger.Fatal("Failed to initialize API: %v", err)
	}
	defer cleanup()

	// Graceful shutdown with timeout
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		logger.Info("Shutting down API server (timeout: %v)...", shutdownTimeout)

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		// Shutdown with timeout - ShutdownWithContext if available
		done := make(chan error, 1)
		go func() {
			done <- app.Shutdown()
		}()

		select {
		case err := <-done:
			if err != nil {
				logger.Error("Error shutting down: %v", err)
			} else {
				logger.Info("API server shut down gracefully")
			}
		case <-ctx.Done():
			logger.Warn("API shutdown timed out, forcing exit")
		}
	}()

	addr := ":" + cfg.Port
	logger.Info("Starting API server on %s", addr)
	if err := app.Listen(addr); err != nil {
		logger.Fatal("Failed to start server: %v", err)
	}
}

func runWorker(cfg *config.Config) {
	worker, cleanup, err := bootstrap.NewWorker(cfg)
	if err != nil {
		logger.Fatal("Failed to initialize worker: %v", err)
	}
	defer cleanup()

	// Graceful shutdown with timeout
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("Shutting down worker (timeout: %v)...", shutdownTimeout)

		// Worker.Stop() already has internal timeout, but we add outer timeout as safety
		done := make(chan struct{})
		go func() {
			worker.Stop()
			close(done)
		}()

		select {
		case <-done:
			logger.Info("Worker shut down gracefully")
		case <-time.After(shutdownTimeout):
			logger.Warn("Worker shutdown timed out, forcing exit")
			os.Exit(1)
		}
	}()

	logger.Info("Starting worker...")
	worker.Start()
}
