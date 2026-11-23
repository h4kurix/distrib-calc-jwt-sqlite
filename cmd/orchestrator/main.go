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
	"calc-service/internal/config"
	"calc-service/internal/database"
	"calc-service/internal/handler"
	"calc-service/internal/logger"
	"calc-service/internal/store"
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

	calculator.Configure(calculator.Limits{
		MaxExpressionLen: cfg.MaxExpressionLen,
		MaxTasksPerExpr:  cfg.MaxTasksPerExpr,
		MaxOperationMS:   cfg.MaxOperationTimeMS,
		AdditionMS:       cfg.TimeAdditionMS,
		SubtractionMS:    cfg.TimeSubtractionMS,
		MultiplicationMS: cfg.TimeMultiplicationMS,
		DivisionMS:       cfg.TimeDivisionMS,
	})

	db, err := database.Open(cfg.DBPath)
	if err != nil {
		logger.Error("database error: %v", err)
		log.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close database: %v", err)
		}
	}()
	deps := handler.NewDeps(store.New(db), cfg.JWTSecret, cfg.AgentTokenTTL, cfg.UserTokenTTL)

	mux := http.NewServeMux()
	// Ограничиваем только публичные роуты аутентификации: bcrypt на каждый
	// запрос делает перебор паролей дорогим для сервера.
	authLimiter := handler.NewRateLimiter(20, time.Minute)
	mux.HandleFunc("/api/v1/register", authLimiter(deps.HandleRegister))
	mux.HandleFunc("/api/v1/login", authLimiter(deps.HandleLogin))

	// Маршрут описан один раз. Параметр {id} доступен обработчику через
	// r.PathValue. Проверку метода делает сам обработчик, чтобы ответ 405
	// оставался JSON.
	user := func(h http.HandlerFunc) http.Handler {
		return deps.AuthMiddleware(h)
	}
	mux.Handle("/api/v1/calculate", user(deps.HandleCalculate))
	mux.Handle("/api/v1/expressions", user(deps.HandleExpressions))
	mux.Handle("/api/v1/expressions/{id}", user(deps.HandleExpressionByID))
	mux.Handle("/api/v1/tasks/{id}", user(deps.HandleTaskByID))

	agent := func(h http.HandlerFunc) http.Handler {
		return deps.AgentAuthMiddleware(h)
	}
	mux.Handle("/internal/task", agent(deps.TaskHandler))
	mux.Handle("/internal/task/result/{id}", agent(deps.HandleInternalTaskByID))
	mux.Handle("/internal/task/fail", agent(deps.HandleTaskFailure))
	mux.HandleFunc("/internal/agent/token", func(w http.ResponseWriter, r *http.Request) {
		handleAgentToken(w, r, cfg, deps)
	})
	// Неизвестные API-пути тоже отвечают JSON, а не статикой FileServer.
	mux.HandleFunc("/api/v1/", handler.HandleNotFound)
	mux.HandleFunc("/internal/", handler.HandleNotFound)

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
	go startMaintenance(ctx, maintenanceInterval, deps)

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

func startMaintenance(ctx context.Context, interval time.Duration, deps *handler.Deps) {
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
				deps.ProcessPendingTasks(ctx)
			}()
		}
	}
}

func handleAgentToken(w http.ResponseWriter, r *http.Request, cfg *config.Config, deps *handler.Deps) {
	if r.Method != http.MethodGet {
		handler.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	provided := r.Header.Get("X-Bootstrap-Key")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.AgentBootstrapKey)) != 1 {
		handler.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}
	token, err := deps.GenerateAgentToken()
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
