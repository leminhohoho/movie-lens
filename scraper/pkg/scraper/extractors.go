package scraper

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/leminhohoho/movie-lens/scraper/pkg/models"
)

// ExtractUsers get all users information from the member page at https://letterboxd.com/members/popular/.
// It return a list of [models.User] and error if the extracting process fails.
func ExtractUsers(doc *goquery.Selection, logger *slog.Logger) ([]models.User, error) {
	users := []models.User{}

	userRows := doc.Find("#content > div > div > section > table > tbody > tr")

	for i := range userRows.Length() {
		node := userRows.Eq(i)

		anchor := node.Find("td > div > h3 > a")

		name := anchor.Text()
		if name == "" {
			return nil, fmt.Errorf("user name can't be empty")
		}

		logger.Debug("user name extracted", "name", name)

		url, exists := anchor.Attr("href")
		if !exists {
			return nil, fmt.Errorf("user url not found for user: %s", name)
		}

		url = "https://letterboxd.com" + url

		users = append(users, models.User{Name: name, Url: url})

		logger.Debug("user url extracted", "name", url)
	}

	return users, nil
}

// ExtractMovieUrls get all movie urls from the user's film page at https://letterboxd.com/[user_name]/films/.
// It return a list of movie urls and error if the extracting process fails.
func ExtractMovieUrls(doc *goquery.Selection, logger *slog.Logger) ([]string, error) {
	urls := []string{}

	filmNodes := doc.Find("#content > div > div > section > div.poster-grid > ul > li")

	for i := range filmNodes.Length() {
		anchor := filmNodes.Eq(i).Find("div > div > a")
		url, exists := anchor.Attr("href")
		if !exists {
			return nil, fmt.Errorf("film url not found")
		}

		url = "https://letterboxd.com" + url

		urls = append(urls, url)

		logger.Debug("movie url extracted", "url", url)
	}

	return urls, nil
}

// ExtractMovie get all movie information from the movie page at https://letterboxd.com/film/[movie_name].
// It return [models.Movie] and error if the extracting process fails.
func ExtractMovie(filmUrl string, doc *goquery.Selection, logger *slog.Logger) (models.Movie, error) {
	movie := models.Movie{Url: filmUrl}

	movie.Name = strings.TrimSpace(doc.Find(
		"#film-page-wrapper > div.col-17 > section.production-masthead.-shadowed.-productionscreen.-film > div > h1 > span").Text(),
	)
	if movie.Name == "" {
		return movie, fmt.Errorf("movie name can't be empty")
	}

	logger.Debug("movie name extracted", "url", movie.Url, "name", movie.Name)

	filmFooterText := strings.TrimSpace(doc.Find("#film-page-wrapper > div.col-17 > section.section.col-10.col-main > p").Text())

	duration, err := strconv.Atoi(strings.Split(filmFooterText, "\u00a0")[0])
	if err != nil {
		logger.Warn("unable to locate movie duration from %s", "footer", filmFooterText)
	} else {
		movie.Duration = &duration

		logger.Debug("movie duration extracted", "url", movie.Url, "duration", *movie.Duration)
	}

	filmPoster := doc.Find("#js-poster-col > section.poster-list.-p230.-single.no-hover.el.col > div.react-component > div > img")
	filmPosterUrl, exists := filmPoster.Attr("src")
	if exists {
		movie.PosterUrl = &filmPosterUrl
	} else {
		logger.Warn("movie does not have poster", "url", movie.Url)
	}

	logger.Debug("movie poster extracted", "url", movie.Url, "poster_url", *movie.PosterUrl)

	filmBackdrop := doc.Find("#backdrop > div.backdropimage.js-backdrop-image")
	filmBackdropStyle, exists := filmBackdrop.Attr("style")
	if exists {
		filmBackdropUrl := regexp.MustCompile(`https:\/\/a\.ltrbxd\.com.+jpg`).FindString(filmBackdropStyle)
		movie.BackdropUrl = &filmBackdropUrl

		logger.Debug("movie backdrop extracted", "url", movie.Url, "backdrop_url", *movie.BackdropUrl)
	} else {
		logger.Warn("movie does not have backdrop", "url", movie.Url)
	}

	return movie, nil
}

// ExtractCasts get all cast information from the movie page at https://letterboxd.com/film/[movie_name].
// It return a list of [models.Crew] and error if extracting process fails.
func ExtractCasts(doc *goquery.Selection, logger *slog.Logger) ([]models.Crew, error) {
	casts := []models.Crew{}

	castNodes := doc.Find(`#tab-cast > div > p > a:not([id="has-cast-overflow"])`)
	hiddenCastNodes := doc.Find(`#tab-cast > div > p > span#cast-overflow > a`)

	castNodes = castNodes.AddSelection(hiddenCastNodes)

	for i := range castNodes.Length() {
		var cast models.Crew

		castNode := castNodes.Eq(i)
		castUrl, exists := castNode.Attr("href")
		if !exists {
			return nil, fmt.Errorf("No cast url found for this actor/actress")
		}
		logger.Debug("cast url extracted", "url", castUrl)

		cast.Name = castNode.Text()
		logger.Debug("cast name extracted", "name", cast.Name)

		cast.Url = "https://letterboxd.com" + castUrl
		cast.Role = "Actor"

		casts = append(casts, cast)
	}

	return casts, nil
}

func ExtractGenresAndThemes(doc *goquery.Selection, logger *slog.Logger) ([]models.Genre, []models.Theme, error) {
	genres := []models.Genre{}
	themes := []models.Theme{}

	categoryLabels := doc.Find("#tab-genres > h3")

	for i := range categoryLabels.Length() {
		categoryLabel := categoryLabels.Eq(i)
		categoryLabelText := strings.TrimSpace(categoryLabel.Text())

		switch categoryLabelText {
		case "Genres":
			genreNodes := categoryLabel.Next().Find("p > a")
			for j := range genreNodes.Length() {
				genreNode := genreNodes.Eq(j)

				genreName := strings.TrimSpace(genreNode.Text())
				if genreName == "" {
					return nil, nil, fmt.Errorf("genre name can't be empty")
				}

				genreUrl, exists := genreNode.Attr("href")
				if !exists {
					return nil, nil, fmt.Errorf("Genre url not found")
				}

				genre := models.Genre{Name: genreName, Url: "https://letterboxd.com" + genreUrl}

				genres = append(genres, genre)
			}
		case "Themes":
			themeNodes := categoryLabel.Next().Find("p > a:not([href^='/film/'])")
			for j := range themeNodes.Length() {
				themeNode := themeNodes.Eq(j)

				themeName := strings.TrimSpace(themeNode.Text())
				if themeName == "" {
					return nil, nil, fmt.Errorf("theme name can't be empty")
				}

				themeUrl, exists := themeNode.Attr("href")
				if !exists {
					return nil, nil, fmt.Errorf("Genre url not found")
				}

				theme := models.Theme{Name: themeName, Url: "https://letterboxd.com" + themeUrl}

				themes = append(themes, theme)
			}
		}
	}

	return genres, themes, nil
}

func ExtractCrews(doc *goquery.Selection, logger *slog.Logger) ([]models.Crew, error) {
	crews := []models.Crew{}

	crewLabels := doc.Find("#tab-crew > h3")

	for i := range crewLabels.Length() {
		role := strings.TrimSpace(crewLabels.Eq(i).Find("span:first-child").Text())
		crewAnchors := crewLabels.Eq(i).Next().Find("p > a")

		for j := range crewAnchors.Length() {
			crewName := strings.TrimSpace(crewAnchors.Eq(j).Text())
			if crewName == "" {
				return nil, fmt.Errorf("crew name can't be empty")
			}

			crewUrl, exists := crewAnchors.Eq(j).Attr("href")
			if !exists {
				return nil, fmt.Errorf("crew url not found")
			}

			crewUrl = "https://letterboxd.com" + crewUrl

			crew := models.Crew{Name: crewName, Url: crewUrl, Role: role}

			crews = append(crews, crew)
		}
	}

	return crews, nil
}
