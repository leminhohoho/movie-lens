package scraper

import (
	"context"
	_ "embed"
	"log/slog"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/leminhohoho/movie-lens/scraper/pkg/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

//go:embed setup.sql
var schema string

type Scraper struct {
	baseCtx context.Context
	db      *gorm.DB
	logger  *slog.Logger
	errChan chan error
}

func NewScraper(cfg config.ScraperConfig, logger *slog.Logger, errChan chan error) (*Scraper, error) {
	var baseCtx context.Context

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = cfg.DbPath
	}

	proxyURL := os.Getenv("PROXY_URL")
	if proxyURL == "" {
		proxyURL = cfg.ProxyURL
	}

	if cfg.BrowserAddr != "" {
		baseCtx, _ = chromedp.NewRemoteAllocator(context.Background(), cfg.BrowserAddr)
	} else {
		baseCtx, _ = chromedp.NewExecAllocator(context.Background(),
			chromedp.Flag("headless", cfg.Headless || os.Getenv("HEADLESS") == "true"),
			chromedp.ProxyServer(proxyURL),
			// NOTE: More options will be added in the future
		)
	}

	var newDB bool

	_, err := os.Stat(dbPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		f, err := os.Create(dbPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		newDB = true
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if newDB {
		if err := db.Exec(schema).Error; err != nil {
			return nil, err
		}
	}

	return &Scraper{
		baseCtx: baseCtx,
		db:      db,
		logger:  logger,
		errChan: errChan,
	}, nil
}
