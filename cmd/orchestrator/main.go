package main

import (
	"context"
	"crypto/subtle"
	"errors"
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
	handler.SetUserTokenTTL(cfg.UserTokenTTL)
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
	// Ограничиваем только публичные роуты аутентификации: bcrypt на каждый
	// запрос делает перебор паролей дорогим для сервера.
	authLimiter := handler.NewRateLimiter(20, time.Minute)
	mux.HandleFunc("/api/v1/register", authLimiter(handler.HandleRegister))
	mux.HandleFunc("/api/v1/login", authLimiter(handler.HandleLogin))

	// Маршрут описан один раз. Параметр {id} доступен обработчику через
	// r.PathValue. Проверку метода делает сам обработчик, чтобы ответ 405
	// оставался JSON.
	user := func(h http.HandlerFunc) http.Handler {
		return handler.AuthMiddleware(h)
	}
	mux.Handle("/api/v1/calculate", user(handler.HandleCalculate))
	mux.Handle("/api/v1/expressions", user(handler.HandleExpressions))
	mux.Handle("/api/v1/expressions/{id}", user(handler.HandleExpressionByID))
	mux.Handle("/api/v1/tasks/{id}", user(handler.HandleTaskByID))

	agent := func(h http.HandlerFunc) http.Handler {
		return handler.AgentAuthMiddleware(h)
	}
	mux.Handle("/internal/task", agent(handler.TaskHandler))
	mux.Handle("/internal/task/result/{id}", agent(handler.HandleInternalTaskByID))
	mux.Handle("/internal/task/fail", agent(handler.HandleTaskFailure))
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

func startMaintenance(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Паника в одном проходе не должна навсегда остановить обслуживание.
			func() {
				defer func() {
					if p := recover(); p != nil {
						logger.Error("maintenance panic: %v", p)
					}
				}()
				handler.ProcessPendingTasks()
			}()
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
	handler.WriteJSON(w, http.StatusOK, struct {
		Token string `json:"token"`
	}{Token: token})
}
