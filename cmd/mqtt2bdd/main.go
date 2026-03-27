package main

import (
	"fmt"
	"log"

	"github.com/spydemon/mqtt2bdd/internal/config"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	fmt.Printf("Configuration loaded: %+v\n", cfg)
}
