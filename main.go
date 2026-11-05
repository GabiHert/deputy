package main

import (
	"embed"
	"log"

	"github.com/GabiHert/deputy/internal/run"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := run.Run(assets); err != nil {
		log.Fatal(err)
	}
}
