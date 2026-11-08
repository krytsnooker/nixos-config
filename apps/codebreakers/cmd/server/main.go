package main

import (
	"log"
	"net/http"

	"codebreakers/internal/config"
	"codebreakers/internal/httpapi"
	"codebreakers/internal/storage"
)

func main() {
	cfg := config.Load()

	store := storage.NewGameStore()
	hub := httpapi.NewHub()
	handler := httpapi.NewHandler(store, hub)
	router := httpapi.NewRouter(handler, cfg.WebDir)

	log.Printf("codebreakers listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, router); err != nil {
		log.Fatal(err)
	}
}
