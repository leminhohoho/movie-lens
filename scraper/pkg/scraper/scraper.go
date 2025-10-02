package scraper

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"regexp"
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
				"content_type", e.Response.MimeType,
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

			s.logger.Debug("user scraped", "user", user)

			if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Table("users").Create(&user).Error; err != nil {
				s.errChan <- err
				return
			}

			s.scrapeUserPage(ctx, user)
		}
	}
}

func (s *Scraper) scrapeUserPage(ctx context.Context, user models.User) {
	if s.db.Table("users").Where("url = ?", user.Url).Limit(1).Find(&[]models.User{}).RowsAffected > 0 {
		return
	}

	var maxFilmsPageStr string
	nextBtnSel := "#content > div > div > section > div.pagination > div:nth-child(2) > a"
	lastPageSel := "#content > div > div > section > div.pagination > div.paginate-pages > ul > li:last-child > a"
	lastMovieSel := "#content > div > div > section > div.poster-grid > ul > li:last-child > div > div > a > span.overlay"

	if err := s.execute(ctx,
		utils.NavigateTillTrigger(user.Url+"films/",
			chromedp.WaitVisible(lastMovieSel),
			utils.Delay(time.Second*2, time.Millisecond*300),
		),
		chromedp.Text(lastPageSel, &maxFilmsPageStr),
	); err != nil {
		s.errChan <- err
		return
	}

	maxFilmsPage, err := strconv.Atoi(strings.TrimSpace(maxFilmsPageStr))
	if err != nil {
		s.errChan <- err
		return
	}

	moviePageCtx, cancel := s.newTab(ctx)
	defer cancel()

	for i := 1; i <= maxFilmsPage; i++ {
		var doc *goquery.Document

		if err := s.execute(ctx,
			chromedp.ActionFunc(func(localCtx context.Context) error {
				if i != 1 {
					return chromedp.Click(nextBtnSel).Do(localCtx)
				}

				return nil
			}),
			chromedp.WaitVisible(lastMovieSel),
			utils.Delay(time.Second*2, time.Millisecond*300),
			utils.ToGoqueryDoc("html", &doc),
		); err != nil {
			s.errChan <- err
			return
		}

		filmNodes := doc.Find("#content > div > div > section > div.poster-grid > ul > li")

		for j := range filmNodes.Length() {
			anchor := filmNodes.Eq(j).Find("div > div > a")
			filmUrl, exists := anchor.Attr("href")
			if !exists {
				s.errChan <- fmt.Errorf("film url not found")
			}

			filmUrl = "https://letterboxd.com" + filmUrl

			s.scrapeMovie(moviePageCtx, filmUrl)
		}
	}
}

func (s *Scraper) scrapeMovie(ctx context.Context, filmUrl string) {
	if s.db.Table("movies").Where("url = ?", filmUrl).Limit(1).Find(&[]models.Movie{}).RowsAffected > 0 {
		return
	}

	var doc *goquery.Document
	var err error

	if err := s.execute(ctx,
		utils.NavigateTillTrigger(filmUrl,
			utils.Delay(time.Second*2, time.Millisecond*300),
			chromedp.ActionFunc(func(localCtx context.Context) error {
				var backdropExists bool

				if err := chromedp.Evaluate(`document.querySelector("#backdrop") != null`, &backdropExists).Do(localCtx); err != nil {
					return err
				}

				if backdropExists {
					return chromedp.WaitVisible(`#backdrop > div.backdropimage.js-backdrop-image[style]:not([style=""])`).Do(localCtx)
				}

				return nil
			}),
			utils.Delay(time.Second*1, time.Millisecond*300),
		),
		utils.ToGoqueryDoc("html", &doc),
	); err != nil {
		s.errChan <- err
		return
	}

	movie := models.Movie{}

	movie.Name = strings.TrimSpace(
		doc.Find("#film-page-wrapper > div.col-17 > section.production-masthead.-shadowed.-productionscreen.-film > div > h1 > span").Text(),
	)
	movie.Url = filmUrl

	filmFooterText := strings.TrimSpace(doc.Find("#film-page-wrapper > div.col-17 > section.section.col-10.col-main > p").Text())

	movie.Duration, err = strconv.Atoi(strings.Split(filmFooterText, "\u00a0")[0])
	if err != nil {
		s.errChan <- err
		return
	}

	filmPoster := doc.Find("#js-poster-col > section.poster-list.-p230.-single.no-hover.el.col > div.react-component > div > img")
	filmPosterSrc, exists := filmPoster.Attr("src")
	if exists {
		movie.PosterUrl = &filmPosterSrc
	}

	filmBackdrop := doc.Find("#backdrop > div.backdropimage.js-backdrop-image")
	filmBackdropStyle, exists := filmBackdrop.Attr("style")
	if exists {
		filmBackdropUrl := regexp.MustCompile(`https:\/\/a\.ltrbxd\.com.+jpg`).FindString(filmBackdropStyle)
		movie.BackdropUrl = &filmBackdropUrl
	}

	s.logger.Debug("movie scraped", "movie", movie)

	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Table("movies").Create(&movie).Error; err != nil {
		s.errChan <- err
		return
	}
}
