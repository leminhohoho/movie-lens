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
			if err := utils.InsertOrUpdate(s.db, s.logger, "users", &users[j], "url = ?", user.Url); err != nil {
				s.errChan <- err
				return
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
		utils.NavigateTillTrigger(user.Url+"films/by/date/",
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

	if err := utils.InsertOrUpdate(s.db, s.logger, "movies", &movie, "url = ?", movie.Url); err != nil {
		s.errChan <- err
		return
	}

	// ---------------- SCRAPE CASTS ----------------- //

	casts, err := ExtractCasts(doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	for i := range casts {
		if err := utils.InsertOrUpdate(s.db, s.logger, "crews", &casts[i], "url = ?", casts[i].Url); err != nil {
			s.errChan <- err
			return
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "crews_and_movies",
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: casts[i].Id},
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: casts[i].Id},
		); err != nil {
			s.errChan <- err
			return
		}
	}

	// ---------------- SCRAPE GENRES & THEMES ----------------- //

	genres, themes, err := ExtractGenresAndThemes(doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	for i := range genres {
		if err := utils.InsertOrUpdate(s.db, s.logger, "genres", &genres[i], "url = ?", genres[i].Url); err != nil {
			s.errChan <- err
			return
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "genres_and_movies",
			&models.GenresAndMovies{MovieId: movie.Id, GenreId: genres[i].Id},
			&models.GenresAndMovies{MovieId: movie.Id, GenreId: genres[i].Id},
		); err != nil {
			s.errChan <- err
			return
		}
	}

	for i := range themes {
		if err := utils.InsertOrUpdate(s.db, s.logger, "themes", &themes[i], "url = ?", themes[i].Url); err != nil {
			s.errChan <- err
			return
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "themes_and_movies",
			&models.ThemesAndMovies{MovieId: movie.Id, ThemeId: themes[i].Id},
			&models.ThemesAndMovies{MovieId: movie.Id, ThemeId: themes[i].Id},
		); err != nil {
			s.errChan <- err
			return
		}
	}

	// ---------------- SCRAPE CREWS ----------------- //

	crews, err := ExtractCrews(doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	for i := range crews {
		if err := utils.InsertOrUpdate(s.db, s.logger, "crews", &crews[i], "url = ?", crews[i].Url); err != nil {
			s.errChan <- err
			return
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "crews_and_movies",
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: crews[i].Id},
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: crews[i].Id},
		); err != nil {
			s.errChan <- err
			return
		}
	}

	// ---------------- SCRAPE STUDIOS ----------------- //

	studios, err := ExtractStudios(doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return

	}

	for i := range studios {
		if err := utils.InsertOrUpdate(s.db, s.logger, "studios", &studios[i], "url = ?", studios[i].Url); err != nil {
			s.errChan <- err
			return
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "studios_and_movies",
			&models.StudiosAndMovies{MovieId: movie.Id, StudioId: studios[i].Id},
			&models.StudiosAndMovies{MovieId: movie.Id, StudioId: studios[i].Id},
		); err != nil {
			s.errChan <- err
			return
		}
	}

	// ---------------- SCRAPE COUNTRIES ----------------- //

	countries, err := ExtractCountries(movie.Id, doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	for i := range countries {
		if err := utils.InsertOrUpdate(
			s.db, s.logger, "countries_and_movies", &countries[i],
			"movie_id = ? AND country = ?",
			countries[i].MovieId, countries[i].Country,
		); err != nil {
			s.errChan <- err
			return
		}
	}

	// ---------------- SCRAPE LANGUAGES ----------------- //

	languages, err := ExtractLanguages(movie.Id, doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	for i := range languages {
		if err := utils.InsertOrUpdate(
			s.db, s.logger, "languages_and_movies", &languages[i],
			"movie_id = ? AND language = ? AND is_primary = ?",
			languages[i].MovieId, languages[i].Language, languages[i].IsPrimary,
		); err != nil {
			s.errChan <- err
			return
		}
	}

	// ---------------- SCRAPE LANGUAGES ----------------- //

	releases, err := ExtractReleases(movie.Id, doc.Selection, s.logger)
	if err != nil {
		s.errChan <- err
		return
	}

	for i := range releases {
		if err := utils.InsertOrUpdate(
			s.db, s.logger, "releases",
			&releases[i],
			&releases[i],
			"movie_id", "date", "release_type", "country", "age_rating",
		); err != nil {
			s.errChan <- err
			return
		}
	}
}
