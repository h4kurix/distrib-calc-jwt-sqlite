package main

import (
	"calc-service/internal/handler"
	"calc-service/pkg/database"
	"calc-service/pkg/logger"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

func main() {
	// Initialize logger
	initLogger()

	// Validate critical environment variables
	if os.Getenv("JWT_SECRET") == "" {
		if os.Getenv("ENV") == "production" {
			logger.Error("FATAL: JWT_SECRET environment variable not set in production!")
			log.Fatal("JWT_SECRET must be set in production")
		} else {
			logger.Error("WARNING: JWT_SECRET environment variable not set. Using an insecure default value for development only!")
			os.Setenv("JWT_SECRET", "insecure-default-secret-"+uuid.New().String())
		}
	}

	// Initialize database
	if err := database.InitDB(); err != nil {
		logger.Error("Failed to initialize database: %v", err)
		log.Fatal(err)
	}
	defer database.CloseDB()

	// Set up router with middleware
	mux := http.NewServeMux()

	// Public authentication endpoints
	mux.HandleFunc("/api/v1/register", handler.HandleRegister)
	mux.HandleFunc("/api/v1/login", handler.HandleLogin)

	// Protected API routes
	// Create a subrouter with auth middleware
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route based on path
		switch {
		case r.URL.Path == "/api/v1/calculate" && r.Method == http.MethodPost:
			handler.HandleCalculate(w, r)
		case r.URL.Path == "/api/v1/expressions" && r.Method == http.MethodGet:
			handler.HandleExpressions(w, r)
		case len(r.URL.Path) > len("/api/v1/expressions/") && r.URL.Path[:len("/api/v1/expressions/")] == "/api/v1/expressions/":
			handler.HandleExpressionByID(w, r)
		case len(r.URL.Path) > len("/api/v1/tasks/") && r.URL.Path[:len("/api/v1/tasks/")] == "/api/v1/tasks/":
			handler.HandleTaskByID(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	// Apply authentication middleware to API routes
	mux.Handle("/api/v1/calculate", handler.AuthMiddleware(apiHandler))
	mux.Handle("/api/v1/expressions", handler.AuthMiddleware(apiHandler))
	mux.Handle("/api/v1/expressions/", handler.AuthMiddleware(apiHandler))
	mux.Handle("/api/v1/tasks/", handler.AuthMiddleware(apiHandler))

	// Internal API for agents (should be protected differently or only accessible internally)
	mux.Handle("/internal/task", handler.AgentAuthMiddleware(http.HandlerFunc(handler.TaskHandler)))
	mux.Handle("/internal/task/result/", handler.AgentAuthMiddleware(http.HandlerFunc(handler.HandleInternalTaskByID)))
	mux.HandleFunc("/internal/agent/token", handleAgentToken)

	// Frontend
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	// Read port from env, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Validate critical environment variables
	if os.Getenv("JWT_SECRET") == "" {
		logger.Error("WARNING: JWT_SECRET environment variable not set. Using an insecure default value!")
		// In production, you might want to exit instead of using a default
		os.Setenv("JWT_SECRET", "insecure-default-secret") // Only for development
	}

	// Start background task to update task readiness periodically
	go startTaskProcessor()

	logger.Info("Server starting on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Error("Server failed: %v", err)
		log.Fatal(err)
	}
}

// Initialize logger
func initLogger() {
	// Dummy initialization to handle the case if logger.Init is not defined
	defer func() {
		if r := recover(); r != nil {
			// If logger.Init panics, fall back to default logger
			log.Println("Warning: Logger initialization failed, using default logger")
		}
	}()

	// Initialize logger
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	logger.Init(logLevel)
}

func startTaskProcessor() {
	// Periodically process tasks
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	logger.Info("Task processor started")

	for {
		select {
		case <-ticker.C:
			// Process tasks for all users
			handler.ProcessPendingTasks()
		}
	}
}

func handleAgentToken(w http.ResponseWriter, r *http.Request) {
	// Проверяем, что запрос выполняется с локального хоста или внутри сети
	remoteIP := r.RemoteAddr
	if !strings.HasPrefix(remoteIP, "127.0.0.1") && !strings.HasPrefix(remoteIP, "10.") &&
		!strings.HasPrefix(remoteIP, "172.") && !strings.HasPrefix(remoteIP, "192.168.") {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Генерируем токен
	token, err := handler.GenerateAgentToken()
	if err != nil {
		http.Error(w, "Failed to generate token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Возвращаем токен в формате JSON
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(map[string]string{"token": token})
	if err != nil {
		http.Error(w, "Failed to encode token: "+err.Error(), http.StatusInternalServerError)
	}
}
