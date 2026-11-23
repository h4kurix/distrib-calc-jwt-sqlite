package handler

import (
	"calc-service/internal/store"
	"time"
)

// Deps содержит зависимости HTTP-слоя. Создаётся один раз при старте сервиса.
type Deps struct {
	Store         *store.Store
	jwtSecret     []byte
	agentTokenTTL time.Duration
	userTokenTTL  time.Duration
}

// NewDeps собирает зависимости из конфигурации.
func NewDeps(st *store.Store, secret string, agentTTL, userTTL time.Duration) *Deps {
	return &Deps{
		Store:         st,
		jwtSecret:     []byte(secret),
		agentTokenTTL: agentTTL,
		userTokenTTL:  userTTL,
	}
}
