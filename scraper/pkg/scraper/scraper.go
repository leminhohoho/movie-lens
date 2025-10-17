package scraper

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/leminhohoho/movie-lens/scraper/pkg/models"
	"github.com/leminhohoho/movie-lens/scraper/pkg/scraper/extractors"
	"github.com/leminhohoho/movie-lens/scraper/pkg/utils"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	prefix = "https://letterboxd.com"
)

//go:embed setup.sql
var schema string

//go:embed jquery.slim.min.js
var jqueryLib string

//go:embed extractors/members_query.js
var memberQuery string

type Scraper struct {
	baseCtx  context.Context
	db       *gorm.DB
	logger   *slog.Logger
	errChan  chan error
	maxPage  int
	interval int
	counter  int
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
	interval, err := strconv.Atoi(os.Getenv("INTERVAL"))
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
		baseCtx:  baseCtx,
		db:       db,
		logger:   logger,
		errChan:  errChan,
		maxPage:  maxPage,
		interval: interval,
	}, nil
}

func (s *Scraper) execute(ctx context.Context, actions ...chromedp.Action) error {
	if s.counter == s.interval {
		time.Sleep(time.Second * 300)
		s.counter = 0
	}

	s.counter++

	return chromedp.Run(ctx, actions...)
}

func (s *Scraper) Run() {
	ctx, cancel, err := utils.NewTab(s.baseCtx, s.logger, utils.InjectLibToCdp(jqueryLib, s.logger))
	if err != nil {
		s.errChan <- err
		return
	}

	defer cancel()

	if err := s.scrapeMembersPages(ctx); err != nil {
		s.errChan <- err
	}
}

func (s *Scraper) scrapeMembersPages(ctx context.Context) error {
	var users []models.User

	for i := range s.maxPage {
		if err := s.execute(ctx,
			utils.NavigateTillTrigger(
				chromedp.Navigate("https://letterboxd.com"+fmt.Sprintf("/members/popular/page/%d/", i+1)), s.logger,
				chromedp.WaitVisible("#content > div > div > section > table > tbody > tr:last-child"),
				utils.Delay(time.Second*2, time.Millisecond*300),
			),
			chromedp.EvaluateAsDevTools(memberQuery, &users, chromedp.EvalAsValue),
		); err != nil {
			return err
		}

		for j := range users {
			if err := utils.InsertOrUpdate(s.db, s.logger, "users", &users[j], "url = ?", users[j].Url); err != nil {
				return err
			}

			if err := s.scrapeUserPage(ctx, users[j]); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Scraper) scrapeUserPage(ctx context.Context, user models.User) error {
	var maxFilmsPageStr string
	nextBtnSel := "#content > div > div > section > div.pagination > div:nth-child(2) > a"
	lastPageSel := "#content > div > div > section > div.pagination > div.paginate-pages > ul > li:last-child > a"
	lastMovieSel := "#content > div > div > section > div.poster-grid > ul > li:last-child > div > div > a > span.overlay"

	if err := s.execute(ctx,
		utils.NavigateTillTrigger(
			chromedp.Navigate(prefix+user.Url+"films/by/date/"), s.logger,
			chromedp.WaitVisible(lastMovieSel),
			utils.Delay(time.Second*2, time.Millisecond*300),
		),
		chromedp.Text(lastPageSel, &maxFilmsPageStr),
	); err != nil {
		return err
	}

	maxFilmsPage, err := strconv.Atoi(strings.TrimSpace(maxFilmsPageStr))
	if err != nil {
		return err
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
			return err
		}
		filmUrls, err := extractors.ExtractMovieUrls(doc.Selection, s.logger)
		if err != nil {
			return err
		}

		for _, filmUrl := range filmUrls {
			moviePageCtx, moviePageCancel, err := utils.NewTab(ctx, s.logger, utils.InjectLibToCdp(jqueryLib, s.logger))
			if err != nil {
				return err
			}

			userActivitiesPageCtx, userActivitiesPageCancel, err := utils.NewTab(
				ctx, s.logger, utils.InjectLibToCdp(jqueryLib, s.logger),
			)
			if err != nil {
				return err
			}

			if err := s.scrapeMovie(moviePageCtx, filmUrl); err != nil {
				return err
			}

			var movie models.Movie

			if err := s.db.Table("movies").Where("url = ?", filmUrl).First(&movie).Error; err != nil {
				return err
			}

			moviePageCancel()

			if err := s.scrapeUserFilmActivities(userActivitiesPageCtx, user, movie); err != nil {
				return err
			}

			userActivitiesPageCancel()
		}
	}

	return nil
}

func (s *Scraper) scrapeMovie(ctx context.Context, filmUrl string) error {
	if s.db.Table("movies").Where("url = ?", filmUrl).Find(&[]models.Movie{}).RowsAffected > 0 {
		s.logger.Warn("movie already in db, skipping", "url", filmUrl)
		return nil
	}

	var doc *goquery.Document

	if err := s.execute(ctx,
		utils.NavigateTillTrigger(
			chromedp.Navigate(prefix+filmUrl), s.logger,
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
		return err
	}

	// ---------------- SCRAPE MOVIE ----------------- //
	movie, err := extractors.ExtractMovie(filmUrl, doc.Selection, s.logger)
	if err != nil {
		return err
	}

	if err := utils.InsertOrUpdate(s.db, s.logger, "movies", &movie, "url = ?", movie.Url); err != nil {
		return err
	}

	// ---------------- SCRAPE CASTS ----------------- //
	casts, err := extractors.ExtractCasts(doc.Selection, s.logger)
	if err != nil {
		return err
	}

	for i := range casts {
		if err := utils.InsertOrUpdate(s.db, s.logger, "crews", &casts[i], "url = ?", casts[i].Url); err != nil {
			return err
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "crews_and_movies",
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: casts[i].Id},
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: casts[i].Id},
		); err != nil {
			return err
		}
	}

	// ---------------- SCRAPE GENRES & THEMES ----------------- //
	genres, themes, err := extractors.ExtractGenresAndThemes(doc.Selection, s.logger)
	if err != nil {
		return err
	}

	for i := range genres {
		if err := utils.InsertOrUpdate(s.db, s.logger, "genres", &genres[i], "url = ?", genres[i].Url); err != nil {
			return err
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "genres_and_movies",
			&models.GenresAndMovies{MovieId: movie.Id, GenreId: genres[i].Id},
			&models.GenresAndMovies{MovieId: movie.Id, GenreId: genres[i].Id},
		); err != nil {
			return err
		}
	}

	for i := range themes {
		if err := utils.InsertOrUpdate(s.db, s.logger, "themes", &themes[i], "url = ?", themes[i].Url); err != nil {
			return err
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "themes_and_movies",
			&models.ThemesAndMovies{MovieId: movie.Id, ThemeId: themes[i].Id},
			&models.ThemesAndMovies{MovieId: movie.Id, ThemeId: themes[i].Id},
		); err != nil {
			return err
		}
	}

	// ---------------- SCRAPE CREWS ----------------- //
	crews, err := extractors.ExtractCrews(doc.Selection, s.logger)
	if err != nil {
		return err
	}

	for i := range crews {
		if err := utils.InsertOrUpdate(s.db, s.logger, "crews", &crews[i], "url = ?", crews[i].Url); err != nil {
			return err
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "crews_and_movies",
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: crews[i].Id},
			&models.CrewsAndMovies{MovieId: movie.Id, CrewId: crews[i].Id},
		); err != nil {
			return err
		}
	}

	// ---------------- SCRAPE STUDIOS ----------------- //
	studios, err := extractors.ExtractStudios(doc.Selection, s.logger)
	if err != nil {
		return err

	}

	for i := range studios {
		if err := utils.InsertOrUpdate(s.db, s.logger, "studios", &studios[i], "url = ?", studios[i].Url); err != nil {
			return err
		}

		if err := utils.InsertOrUpdate(
			s.db, s.logger, "studios_and_movies",
			&models.StudiosAndMovies{MovieId: movie.Id, StudioId: studios[i].Id},
			&models.StudiosAndMovies{MovieId: movie.Id, StudioId: studios[i].Id},
		); err != nil {
			return err
		}
	}

	// ---------------- SCRAPE COUNTRIES ----------------- //
	countries, err := extractors.ExtractCountries(movie.Id, doc.Selection, s.logger)
	if err != nil {
		return err
	}

	for i := range countries {
		if err := utils.InsertOrUpdate(
			s.db, s.logger, "countries_and_movies", &countries[i],
			"movie_id = ? AND country = ?",
			countries[i].MovieId, countries[i].Country,
		); err != nil {
			return err
		}
	}

	// ---------------- SCRAPE LANGUAGES ----------------- //
	languages, err := extractors.ExtractLanguages(movie.Id, doc.Selection, s.logger)
	if err != nil {
		return err
	}

	for i := range languages {
		if err := utils.InsertOrUpdate(
			s.db, s.logger, "languages_and_movies", &languages[i],
			"movie_id = ? AND language = ? AND is_primary = ?",
			languages[i].MovieId, languages[i].Language, languages[i].IsPrimary,
		); err != nil {
			return err
		}
	}

	// ---------------- SCRAPE LANGUAGES ----------------- //
	releases, err := extractors.ExtractReleases(movie.Id, doc.Selection, s.logger)
	if err != nil {
		return err
	}

	for i := range releases {
		if err := utils.InsertOrUpdate(
			s.db, s.logger, "releases",
			&releases[i],
			&releases[i],
			"movie_id", "date", "release_type", "country", "age_rating",
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *Scraper) scrapeUserFilmActivities(ctx context.Context, user models.User, movie models.Movie) error {
	url := prefix + path.Join(user.Url, movie.Url, "/activity")

	var doc *goquery.Document

	if err := s.execute(ctx,
		utils.NavigateTillTrigger(
			chromedp.Navigate(url), s.logger,
			utils.Delay(time.Second*2, time.Millisecond*300),
			chromedp.WaitVisible("#activity-table-body > section.activity-row.no-activity-message > p"),
			utils.Delay(time.Second*1, time.Millisecond*300),
		),
		utils.ToGoqueryDoc("html", &doc),
	); err != nil {
		return err
	}

	activityNodes := doc.Find("#activity-table-body > section[data-activity-id]")

	for i := range activityNodes.Length() {
		node := activityNodes.Eq(i)
		userAndMovie := models.UserAndMovie{UserId: user.Id, MovieId: movie.Id}

		activityDateStr, exists := node.Find("time").Attr("datetime")
		if !exists {
			s.logger.Warn("activity date not found, skipping", "user", user.Url, "movie", movie.Url)
			continue
		}

		userAndMovie.Date = strings.TrimSpace(activityDateStr)

		content := node.Find("div > p > a > span.context").Text()

		actions := utils.MultiSplit(content, "and", ",")

		for i := range actions {
			action := strings.TrimSpace(actions[i])

			switch action {
			case "liked":
				userAndMovie.IsLoved = true
			case "watched", "rewatched":
				userAndMovie.IsWatch = true
			case "rated":
				ratingStr := node.Find("div > p > span.rating").Text()
				if ratingStr != "" {
					rating := float32(strings.Count(ratingStr, "★")) + float32(strings.Count(ratingStr, "½"))/2
					userAndMovie.Rating = &rating
				}
			case "reviewed":
				reviewUrl, exists := node.Find("div > p > a.target").Attr("href")
				if !exists {
					s.logger.Warn("review url is empty, skipping")
					continue
				}

				reviewPageCtx, reviewPageCancel, err := utils.NewTab(ctx, s.logger, utils.InjectLibToCdp(jqueryLib, s.logger))
				if err != nil {
					return err
				}

				review, err := s.scrapeUserReviewPage(reviewPageCtx, reviewUrl)
				if err != nil {
					return err
				}

				reviewPageCancel()

				if review != "" {
					userAndMovie.Review = &review
				}
			}
		}

		if err := utils.InsertOrUpdate(s.db, s.logger,
			"users_and_movies",
			&userAndMovie,
			&userAndMovie,
			"user_id", "movie_id", "date",
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *Scraper) scrapeUserReviewPage(ctx context.Context, reviewUrl string) (string, error) {
	var review string

	reviewContentSel := "#content > div > div > section > section > div.review.body-text.-prose.-hero.-loose > div > div > div > p"
	moviePosterSel := "#content > div > div > section > div.col-4.gutter-right-1 > section.poster-list.-p150.el.col.viewing-poster-container > div > div > a > span.overlay"
	spoilerBtnSel := "#content > div > div > section > section > div.review.body-text.-prose.-hero.-loose > div.js-spoiler-container > div > div > a"

	if err := s.execute(ctx,
		utils.NavigateTillTrigger(
			chromedp.Navigate(prefix+reviewUrl), s.logger,
			utils.Delay(time.Second*2, time.Millisecond*300),
			chromedp.WaitVisible(moviePosterSel),
			utils.Delay(time.Second*1, time.Millisecond*300),
		),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var spoilerAlert bool

			if err := chromedp.Evaluate(
				fmt.Sprintf(`document.querySelector("%s") != null`, spoilerBtnSel), &spoilerAlert,
			).Do(ctx); err != nil {
				return err
			}

			if spoilerAlert {
				if err := chromedp.Click(spoilerBtnSel).Do(ctx); err != nil {
					return err
				}
			}

			return nil
		}),
		chromedp.Text(reviewContentSel, &review),
	); err != nil {
		return "", err
	}

	return review, nil
}
