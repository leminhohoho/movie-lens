package scraper

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/leminhohoho/movie-lens/scraper/pkg/models"
	"github.com/leminhohoho/movie-lens/scraper/pkg/utils"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed setup.sql
var schema string

type Scraper struct {
	baseCtx context.Context
	db      *gorm.DB
	logger  *slog.Logger
	errChan chan error
	maxPage int
}

func NewScraper(logger *slog.Logger, errChan chan error) (*Scraper, error) {
	var baseCtx context.Context

	dbPath := os.Getenv("DB_PATH")
	proxyURL := os.Getenv("PROXY_URL")
	browserAddr := os.Getenv("BROWSER_ADDR")
	userDataDir := os.Getenv("USER_DATA_DIR")
	maxPage, err := strconv.Atoi(os.Getenv("MAX_PAGE"))
	if err != nil {
		return nil, err
	}

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

	baseCtx, _ = chromedp.NewContext(baseCtx)

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
		maxPage: maxPage,
	}, nil
}

func (s *Scraper) Run() {
	ctx, cancel := s.newTab(s.baseCtx)
	defer cancel()

	s.scrapeMembersPages(ctx)
}

func (s *Scraper) newTab(ctx context.Context) (context.Context, context.CancelFunc) {
	cdpCtx, cancel := chromedp.NewContext(ctx)

	go chromedp.ListenTarget(cdpCtx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			s.logger.Debug(
				"request to be sent",
				"url", e.Request.URL,
				"method", e.Request.Method,
			)
		case *network.EventResponseReceived:
			s.logger.Debug(
				"response recieved",
				"url", e.Response.URL,
				"status_code", e.Response.Status,
			)
		}
	})

	return cdpCtx, cancel
}

func (s *Scraper) execute(ctx context.Context, tasks ...chromedp.Action) error {
	return chromedp.Run(ctx, tasks...)
}

func (s *Scraper) scrapeMembersPages(ctx context.Context) {
	for i := range s.maxPage {
		var doc *goquery.Document

		if err := s.execute(ctx,
			utils.NavigateTillTrigger(fmt.Sprintf("https://letterboxd.com/members/popular/page/%d/", i+1),
				chromedp.WaitVisible("#content > div > div > section > table > tbody > tr:last-child"),
				utils.Delay(time.Second*2, time.Millisecond*300),
			),
			utils.ToGoqueryDoc("html", &doc),
		); err != nil {
			s.errChan <- err
			return
		}

		userRows := doc.Find("#content > div > div > section > table > tbody > tr")

		for i := range userRows.Length() {
			node := userRows.Eq(i)

			anchor := node.Find("td > div > h3 > a")

			url, exists := anchor.Attr("href")
			if !exists {
				s.errChan <- fmt.Errorf("Attribute not exists")
				return
			}

			user := models.User{
				Url:  "https://letterboxd.com" + strings.TrimSpace(url),
				Name: strings.TrimSpace(anchor.Text()),
			}

			s.logger.Debug(
				"user scraped",
				"url", user.Url,
				"name", user.Name,
			)

			if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Table("users").Create(&user).Error; err != nil {
				s.errChan <- err
				return
			}
		}
	}
}
