package config

import "os"

type Config struct {
	Addr     string
	WebDir   string
}

func Load() Config {
	cfg := Config{
		Addr:   ":8099",
		WebDir: "web",
	}
	if v := os.Getenv("CODEBREAKERS_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("CODEBREAKERS_WEB_DIR"); v != "" {
		cfg.WebDir = v
	}
	return cfg
}
