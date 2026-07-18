package main

import (
	"log"
	"os"

	"shh-h/internal/app"
	"shh-h/internal/frontendassets"
	"shh-h/internal/terminalbenchmark"
)

func main() {
	handled, err := terminalbenchmark.RunFixtureIfRequested(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
	if handled {
		return
	}
	assets, err := frontendassets.Assets()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(assets); err != nil {
		log.Fatal(err)
	}
}
