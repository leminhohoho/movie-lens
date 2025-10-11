package scraper

import (
	"testing"

	"github.com/joho/godotenv"
	"github.com/leminhohoho/movie-lens/scraper/pkg/logger"
	"github.com/leminhohoho/movie-lens/scraper/pkg/utils"
)

func TestMovieScraper(t *testing.T) {
	if err := godotenv.Load("../../.env"); err != nil {
		t.Fatal(err)
	}

	errChan := make(chan error)
	doneChan := make(chan bool)

	l, err := logger.NewLogger()
	if err != nil {
		t.Fatal(err)
	}

	scp, err := NewScraper(l, errChan)
	if err != nil {
		t.Fatal(err)
	}

	cdpCtx, cancel := utils.NewTab(scp.baseCtx, l)
	defer cancel()

	go func() {
		scp.scrapeMovie(cdpCtx, "/film/godzilla-kong-the-new-empire/crew/")
		doneChan <- true
	}()

	select {
	case err := <-errChan:
		t.Fatal(err)
	case <-doneChan:
		return
	}
}
