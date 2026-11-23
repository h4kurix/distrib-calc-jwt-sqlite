package handler

import (
	"calc-service/internal/store"
	"time"
)

// Options собирает настройки HTTP-слоя из конфигурации сервиса.
type Options struct {
	Secret           string
	AgentTokenTTL    time.Duration
	UserTokenTTL     time.Duration
	StaleTaskTimeout time.Duration
	ExpressionExpiry time.Duration
}

// Deps содержит зависимости HTTP-слоя. Создаётся один раз при старте сервиса.
type Deps struct {
	Store            *store.Store
	jwtSecret        []byte
	agentTokenTTL    time.Duration
	userTokenTTL     time.Duration
	staleTaskTimeout time.Duration
	expressionExpiry time.Duration
}

// NewDeps собирает зависимости из конфигурации.
func NewDeps(st *store.Store, o Options) *Deps {
	return &Deps{
		Store:            st,
		jwtSecret:        []byte(o.Secret),
		agentTokenTTL:    o.AgentTokenTTL,
		userTokenTTL:     o.UserTokenTTL,
		staleTaskTimeout: o.StaleTaskTimeout,
		expressionExpiry: o.ExpressionExpiry,
	}
}
