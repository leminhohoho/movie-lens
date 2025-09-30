package scraper

import (
	"context"
	_ "embed"
	"log/slog"
	"os"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
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

func NewScraper(logger *slog.Logger, errChan chan error) (*Scraper, error) {
	var baseCtx context.Context

	dbPath := os.Getenv("DB_PATH")
	proxyURL := os.Getenv("PROXY_URL")
	browserAddr := os.Getenv("BROWSER_ADDR")
	userDataDir := os.Getenv("USER_DATA_DIR")

	if browserAddr != "" {
		baseCtx, _ = chromedp.NewRemoteAllocator(context.Background(), browserAddr)
	} else {
		opts := []func(*chromedp.ExecAllocator){
			chromedp.Flag("headless", os.Getenv("HEADLESS") == "TRUE"),
			// NOTE: More options will be added in the future
		}

		if proxyURL != "" {
			opts = append(opts, chromedp.ProxyServer(proxyURL))
		}

		if userDataDir != "" {
			opts = append(opts, chromedp.UserDataDir(userDataDir))
		}

		baseCtx, _ = chromedp.NewExecAllocator(context.Background(), opts...)
	}

	var newDB bool

	if _, err := os.Stat(dbPath); err != nil {
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

func (s *Scraper) Run() error {
	ctx, cancel := chromedp.NewContext(s.baseCtx)
	defer cancel()

	if err := s.execute(ctx,
		chromedp.Navigate("https://letterboxd.com/members/popular/page/1/"),
		chromedp.ActionFunc(func(ctx context.Context) error { time.Sleep(time.Second * 5); return nil }),
	); err != nil {
		return err
	}

	return nil
}

func (s *Scraper) execute(ctx context.Context, tasks ...chromedp.Action) error {
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			s.logger.Debug(
				"request to be sent",
				"url", e.Request.URL,
				"method", e.Request.Method,
				"headers", e.Request.Headers,
			)
		case *network.EventResponseReceived:
			s.logger.Debug(
				"response recieved",
				"url", e.Response.URL,
				"status_code", e.Response.Status,
				"headers", e.Response.Headers,
			)
		}
	})

	return chromedp.Run(ctx, tasks...)
}
