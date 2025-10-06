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

		users, err := ExtractUsers(doc.Selection, s.logger)
		if err != nil {
			s.errChan <- err
			return
		}

		for j, user := range users {
			if s.db.Table("users").Where("url = ?", user.Url).Find(&[]models.Movie{}).RowsAffected > 0 {
				if err := s.db.Table("users").Where("url = ?", user.Url).First(&users[j]).Error; err != nil {
					s.errChan <- err
					return
				}

				s.logger.Warn("user is already in the database", "user", user)
			} else {
				if err := s.db.Table("users").Create(&user).Error; err != nil {
					s.errChan <- err
					return
				}

				s.logger.Info("new user added to db", "user", user)
			}

			s.scrapeUserPage(ctx, user)
		}
	}
}

func (s *Scraper) scrapeUserPage(ctx context.Context, user models.User) {
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

		filmUrls, err := ExtractMovieUrls(doc.Selection, s.logger)
		if err != nil {
			s.errChan <- err
			return
		}

		for _, filmUrl := range filmUrls {
			moviePageCtx, cancel := s.newTab(ctx)
			s.scrapeMovie(moviePageCtx, filmUrl)
			cancel()
		}
	}
}

func (s *Scraper) scrapeMovie(ctx context.Context, filmUrl string) {
	var doc *goquery.Document

	if err := s.execute(ctx,
		utils.NavigateTillTrigger(filmUrl,
			utils.Delay(time.Second*2, time.Millisecond*300),
			chromedp.ActionFunc(func(localCtx context.Context) error {
				var backdropExists bool

				if err := chromedp.Evaluate(`document.querySelector("#backdrop") != null`, &backdropExists).Do(localCtx); err != nil {
					return err
				}

				if backdropExists {
					return chromedp.Tasks{
						chromedp.WaitVisible(`body.backdrop-loaded`),
						chromedp.WaitVisible(`#js-poster-col > section.poster-list.-p230.-single.no-hover.el.col > div.react-component > div > img[srcset]`),
					}.Do(localCtx)
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

	// ---------------- SCRAPE MOVIE ----------------- //

	movie, err := ExtractMovie(filmUrl, doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	if s.db.Table("movies").Where("url = ?", movie.Url).Find(&[]models.Crew{}).RowsAffected > 0 {
		if err := s.db.Table("movies").Where("url = ?", movie.Url).First(&movie).Error; err != nil {
			s.errChan <- err
			return
		}

		s.logger.Warn("movie already in the database", "movie", movie)
	} else {
		if err := s.db.Table("movies").Create(&movie).Error; err != nil {
			s.errChan <- err
			return
		}

		s.logger.Info("new movie added to db", "movie", movie)
	}

	// ---------------- SCRAPE CASTS ----------------- //

	casts, err := ExtractCasts(doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	crewTable := s.db.Table("crews")

	for i, cast := range casts {
		if crewTable.Where("url = ?", cast.Url).Find(&[]models.Crew{}).RowsAffected > 0 {
			if err := crewTable.Where("url = ?", cast.Url).First(&casts[i]).Error; err != nil {
				s.errChan <- err
				return
			}

			s.logger.Warn("cast already in the database", "cast", cast)
		} else {
			if err := crewTable.Create(&cast).Error; err != nil {
				s.errChan <- err
				return
			}

			s.logger.Debug("new cast added to db", "cast", cast)
		}
	}

	// ---------------- SCRAPE GENRES & THEMES ----------------- //

	genres, themes, err := ExtractGenresAndThemes(doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	for i, genre := range genres {
		if s.db.Table("genres").Where("url = ?", genre.Url).Find(&[]models.Crew{}).RowsAffected > 0 {
			if err := s.db.Table("genres").Where("url = ?", genre.Url).First(&genres[i]).Error; err != nil {
				s.errChan <- err
				return
			}

			s.logger.Warn("genre already in the database", "genre", genre)
		} else {
			if err := s.db.Table("genres").Create(&genre).Error; err != nil {
				s.errChan <- err
				return
			}

			s.logger.Info("new genre added to db", "genre", genre)
		}
	}

	for i, theme := range themes {
		if s.db.Table("themes").Where("url = ?", theme.Url).Find(&[]models.Crew{}).RowsAffected > 0 {
			if err := s.db.Table("themes").Where("url = ?", theme.Url).First(&themes[i]).Error; err != nil {
				s.errChan <- err
				return
			}

			s.logger.Warn("theme already in the database", "theme", theme)
		} else {
			if err := s.db.Table("themes").Create(&theme).Error; err != nil {
				s.errChan <- err
				return
			}

			s.logger.Info("new theme added to db", "theme", theme)
		}
	}

	// ---------------- SCRAPE CREWS ----------------- //

	crewLabels := doc.Find("#tab-crew > h3")

	for i := range crewLabels.Length() {
		role := strings.TrimSpace(crewLabels.Eq(i).Find("span:first-child").Text())
		crewAnchors := crewLabels.Eq(i).Next().Find("p > a")

		for j := range crewAnchors.Length() {
			crewName := strings.TrimSpace(crewAnchors.Eq(j).Text())
			crewUrl, exists := crewAnchors.Eq(j).Attr("href")
			if !exists {
				s.errChan <- fmt.Errorf("crew url not found")
				return
			}

			crewUrl = "https://letterboxd.com" + crewUrl

			crew := models.Crew{Name: crewName, Url: crewUrl, Role: role}

			if err := s.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "url"}},
				DoUpdates: clause.Assignments(map[string]interface{}{"url": gorm.Expr("excluded.url")}),
			}).Table("crews").Create(&crew).Error; err != nil {
				s.errChan <- err
				return
			}

			s.logger.Debug("crew scraped", "crew", crew)
		}
	}
}
