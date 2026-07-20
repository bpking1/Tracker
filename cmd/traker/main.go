package main

import (
	"bufio"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"traker/internal/server"
	"traker/internal/store"
)

//go:embed web/*
var webAssets embed.FS

func main() {
	if err := loadEnvFile(".env"); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: cannot read .env: %v", err)
	}
	defaultData := envOr("TRAKER_DATA_FILE", filepath.Join("data", "traker.txt"))
	addr := flag.String("addr", envOr("TRAKER_ADDR", "127.0.0.1:8080"), "listen address")
	dataFile := flag.String("data", defaultData, "path to traker text file")
	flag.Parse()

	recordStore, err := store.New(*dataFile)
	if err != nil {
		log.Fatal(err)
	}

	var static fs.FS
	if sub, err := fs.Sub(webAssets, "web"); err == nil {
		static = sub
	}

	handler, err := server.New(recordStore, static)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Traker listening at http://%s (data: %s)", *addr, *dataFile)
	if err := http.ListenAndServe(*addr, handler); err != nil {
		log.Fatal(err)
	}
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		_ = os.Setenv(key, value)
	}
	return scanner.Err()
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
