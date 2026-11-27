package main

import (
	"errors"
	"strings"
	"time"
)

func configuredAICacheTTL(getenv func(string) string) (time.Duration, error) {
	value := strings.TrimSpace(getenv("PROJECT_CREATOR_AI_CACHE_TTL"))
	if value == "" {
		return 0, nil
	}
	ttl, err := time.ParseDuration(value)
	if err != nil || ttl < 0 {
		return 0, errors.New("PROJECT_CREATOR_AI_CACHE_TTL must be a non-negative duration such as 24h")
	}
	return ttl, nil
}
