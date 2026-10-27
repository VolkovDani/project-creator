package main

import (
	"github.com/VolkovDani/project-creator/internal/api"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	token := os.Getenv("PROJECT_CREATOR_TOKEN")
	if len(token) < 32 {
		log.Fatal("PROJECT_CREATOR_TOKEN must have at least 32 characters")
	}
	path := os.Getenv("PROJECT_CREATOR_DB")
	if path == "" {
		path = "project-creator.db"
	}
	addr := os.Getenv("PROJECT_CREATOR_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if strings.TrimSpace(token) != token {
		log.Fatal("token must not contain edge whitespace")
	}
	s, err := api.New(path, map[string]string{token: "default"})
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	log.Fatal(http.ListenAndServe(addr, s))
}
