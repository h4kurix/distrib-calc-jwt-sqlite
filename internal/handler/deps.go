package handler

import "time"

// Deps содержит зависимости HTTP-слоя. Создаётся один раз при старте сервиса.
type Deps struct {
	jwtSecret     []byte
	agentTokenTTL time.Duration
	userTokenTTL  time.Duration
}

// NewDeps собирает зависимости из конфигурации.
func NewDeps(secret string, agentTTL, userTTL time.Duration) *Deps {
	return &Deps{
		jwtSecret:     []byte(secret),
		agentTokenTTL: agentTTL,
		userTokenTTL:  userTTL,
	}
}
