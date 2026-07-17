package main

import (
	"log"

	"shh-h/internal/app"
	"shh-h/internal/frontendassets"
)

func main() {
	assets, err := frontendassets.Assets()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(assets); err != nil {
		log.Fatal(err)
	}
}
