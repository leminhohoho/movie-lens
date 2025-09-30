package app

import (
	"log/slog"
	"os"

	"github.com/leminhohoho/movie-lens/scraper/pkg/logger"
	"github.com/leminhohoho/movie-lens/scraper/pkg/scraper"
)

type App struct {
	Logger  *slog.Logger
	Scraper *scraper.Scraper

	ErrChan chan error
}

func NewApp() (*App, error) {
	var err error

	app := &App{
		ErrChan: make(chan error),
	}

	app.Logger, err = logger.NewLogger()
	if err != nil {
		return nil, err
	}

	app.Scraper, err = scraper.NewScraper(app.Logger, app.ErrChan)
	if err != nil {
		return nil, err
	}

	return app, nil
}

func (a *App) Run() error {
	a.Logger.Debug(
		"scraper info",
		"db_path", os.Getenv("DB_PATH"),
		"proxy_url", os.Getenv("PROXY_URL"),
		"headless", os.Getenv("HEADLESS") == "TRUE",
		"browser_addr", os.Getenv("BROWSER_ADDR"),
		"user_data_dir", os.Getenv("USER_DATA_DIR"),
		"debug", os.Getenv("DEBUG") == "TRUE",
		"silent", os.Getenv("SILENT") == "TRUE",
	)

	go a.Scraper.Run()

	for {
		select {
		case err := <-a.ErrChan:
			a.Logger.Error(err.Error())
		}
	}
}

func (a *App) Close() {
	close(a.ErrChan)
}
