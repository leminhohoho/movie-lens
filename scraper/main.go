package main

import (
	"log"

	"github.com/joho/godotenv"
	"github.com/leminhohoho/movie-lens/scraper/pkg/app"
)

func main() {
	if err := godotenv.Load(".env"); err != nil {
		log.Fatal(err)
	}

	app, err := app.NewApp()
	if err != nil {
		log.Fatal(err)
	}

	err = app.Run()
	app.Close()

	if err != nil {
		log.Fatal(err)
	}
}
