package main

import (
	"os"

	"github.com/Fadil-Tao/paddock/internal/cli"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	baseURL := os.Getenv("PADDOCK_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}

	client := cli.NewClient(baseURL)
	os.Exit(cli.Execute(client, os.Args[1:]))
}
