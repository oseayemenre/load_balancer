package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port int
}

func FromEnv() *Config {
	return &Config{
		Port: getEnvInt("PORT", 3000),
	}
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return fallback
		}
		return parsed
	}
	return fallback
}

func (c *Config) Validate() error {
	return nil
}
