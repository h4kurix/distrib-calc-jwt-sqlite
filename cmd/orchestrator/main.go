package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"calc-service/internal/calculator"
	"calc-service/internal/handler"
	"calc-service/pkg/config"
	"calc-service/pkg/database"
	"calc-service/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	if err := logger.Init(cfg.Env, cfg.LogLevel); err != nil {
		log.Fatalf("logger error: %v", err)
	}
	defer logger.Sync()

	handler.SetJWTSecret(cfg.JWTSecret)
	handler.SetAgentTokenTTL(cfg.AgentTokenTTL)
	calculator.Configure(cfg.MaxExpressionLen, cfg.MaxTasksPerExpr, cfg.MaxOperationTimeMS)

	if err := database.InitDB(cfg.DBPath); err != nil {
		logger.Error("database error: %v", err)
		log.Fatal(err)
	}
	defer func() {
		if err := database.CloseDB(); err != nil {
			logger.Error("close database: %v", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/register", handler.HandleRegister)
	mux.HandleFunc("/api/v1/login", handler.HandleLogin)

	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/calculate" && r.Method == http.MethodPost:
			handler.HandleCalculate(w, r)
		case r.URL.Path == "/api/v1/expressions" && r.Method == http.MethodGet:
			handler.HandleExpressions(w, r)
		case hasPrefix(r.URL.Path, "/api/v1/expressions/"):
			handler.HandleExpressionByID(w, r)
		case hasPrefix(r.URL.Path, "/api/v1/tasks/"):
			handler.HandleTaskByID(w, r)
		default:
			handler.HandleNotFound(w)
		}
	})
	mux.Handle("/api/v1/calculate", handler.AuthMiddleware(apiHandler))
	mux.Handle("/api/v1/expressions", handler.AuthMiddleware(apiHandler))
	mux.Handle("/api/v1/expressions/", handler.AuthMiddleware(apiHandler))
	mux.Handle("/api/v1/tasks/", handler.AuthMiddleware(apiHandler))

	mux.Handle("/internal/task", handler.AgentAuthMiddleware(http.HandlerFunc(handler.TaskHandler)))
	mux.Handle("/internal/task/result/", handler.AgentAuthMiddleware(http.HandlerFunc(handler.HandleInternalTaskByID)))
	mux.Handle("/internal/task/fail", handler.AgentAuthMiddleware(http.HandlerFunc(handler.HandleTaskFailure)))
	mux.HandleFunc("/internal/agent/token", func(w http.ResponseWriter, r *http.Request) {
		handleAgentToken(w, r, cfg)
	})

	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "./static"
	}
	if _, err := os.Stat(staticDir); err != nil {
		logger.Warn("static directory %q is not available: %v", staticDir, err)
	}
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Обслуживание очереди — редкая операция. Частый тикер зря нагружает WAL.
	maintenanceInterval := cfg.TaskPollInterval
	if maintenanceInterval < 2*time.Second {
		maintenanceInterval = 2 * time.Second
	}
	go startMaintenance(ctx, maintenanceInterval)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	logger.Info("orchestrator listening on %s", srv.Addr)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-stop:
		logger.Info("received %s, shutting down", sig)
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed: %v", err)
			log.Fatal(err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown: %v", err)
	}
}

func hasPrefix(path, prefix string) bool {
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}

func startMaintenance(ctx context.Context, interval time.Duration) {
	defer func() {
		if p := recover(); p != nil {
			logger.Error("maintenance panic: %v", p)
		}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			handler.ProcessPendingTasks()
		}
	}
}

func handleAgentToken(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	if r.Method != http.MethodGet {
		handler.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	provided := r.Header.Get("X-Bootstrap-Key")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.AgentBootstrapKey)) != 1 {
		handler.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	token, err := handler.GenerateAgentToken()
	if err != nil {
		logger.Error("generate agent token: %v", err)
		handler.WriteError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeToken(w, token)
}

func writeToken(w http.ResponseWriter, token string) {
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprintf(w, "{\"token\":%q}\n", token); err != nil {
		logger.Error("write token: %v", err)
	}
}
