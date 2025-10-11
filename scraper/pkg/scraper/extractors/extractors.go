package extractors

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

		cast.Url = castUrl
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

				genre := models.Genre{Name: genreName, Url: genreUrl}

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

				theme := models.Theme{Name: themeName, Url: themeUrl}

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
		logger.Debug("crew role", "role", role)

		crewAnchors := crewLabels.Eq(i).Next().Find("p > a")

		for j := range crewAnchors.Length() {
			crewName := strings.TrimSpace(crewAnchors.Eq(j).Text())
			if crewName == "" {
				logger.Warn("crew name can't be empty")
				continue
			}

			crewUrl, exists := crewAnchors.Eq(j).Attr("href")
			if !exists {
				return nil, fmt.Errorf("crew url not found")
			}

			crew := models.Crew{Name: crewName, Url: crewUrl, Role: role}

			crews = append(crews, crew)
		}
	}

	return crews, nil
}

func ExtractStudios(doc *goquery.Selection, logger *slog.Logger) ([]models.Studio, error) {
	studios := []models.Studio{}

	detailLabels := doc.Find("#tab-details > h3")

	for i := range detailLabels.Length() {
		detailLabel := detailLabels.Eq(i)

		detailName := strings.TrimSpace(detailLabel.Find("span:first-child").Text())
		if detailName != "Studios" {
			continue
		}

		studioAnchors := detailLabel.Next().Find("p > a")

		for j := range studioAnchors.Length() {
			studioAnchor := studioAnchors.Eq(j)

			studioName := strings.TrimSpace(studioAnchor.Text())
			if studioName == "" {
				return nil, fmt.Errorf("studio name can't be empty")
			}

			studioUrl, exists := studioAnchor.Attr("href")
			if !exists {
				return nil, fmt.Errorf("studio url not found")
			}

			studio := models.Studio{Name: studioName, Url: studioUrl}

			studios = append(studios, studio)
		}
	}

	return studios, nil
}

func ExtractCountries(movieId int, doc *goquery.Selection, logger *slog.Logger) ([]models.CountriesAndMovies, error) {
	countries := []models.CountriesAndMovies{}

	detailLabels := doc.Find("#tab-details > h3")

	for i := range detailLabels.Length() {
		detailLabel := detailLabels.Eq(i)

		detailName := strings.TrimSpace(detailLabel.Find("span:first-child").Text())
		if detailName != "Countries" && detailName != "Country" {
			continue
		}

		countryAnchors := detailLabel.Next().Find("p > a")

		for j := range countryAnchors.Length() {

			countryName := strings.TrimSpace(countryAnchors.Eq(j).Text())
			if countryName == "" {
				return nil, fmt.Errorf("country name can't be empty")
			}

			countries = append(countries, models.CountriesAndMovies{MovieId: movieId, Country: countryName})
		}
	}

	return countries, nil
}

func ExtractLanguages(movieId int, doc *goquery.Selection, logger *slog.Logger) ([]models.LanguagesAndMovies, error) {
	languages := []models.LanguagesAndMovies{}

	detailLabels := doc.Find("#tab-details > h3")

	for i := range detailLabels.Length() {
		detailLabel := detailLabels.Eq(i)
		detailName := strings.TrimSpace(detailLabel.Find("span:first-child").Text())
		languageAnchors := detailLabel.Next().Find("p > a")

		switch detailName {
		case "Language", "Primary Language", "Languages", "Primary Languages":
			for j := range languageAnchors.Length() {

				languageName := strings.TrimSpace(languageAnchors.Eq(j).Text())
				if languageName == "" {
					return nil, fmt.Errorf("language name can't be empty")
				}

				languages = append(languages, models.LanguagesAndMovies{MovieId: movieId, Language: languageName, IsPrimary: true})
			}
		case "Spoken Languages", "Spoken Language":
			for j := range languageAnchors.Length() {

				languageName := strings.TrimSpace(languageAnchors.Eq(j).Text())
				if languageName == "" {
					return nil, fmt.Errorf("language name can't be empty")
				}

				languages = append(languages, models.LanguagesAndMovies{MovieId: movieId, Language: languageName, IsPrimary: false})
			}
		}
	}

	return languages, nil
}

func ExtractReleases(movieId int, doc *goquery.Selection, logger *slog.Logger) ([]models.Release, error) {
	releases := []models.Release{}

	releaseLabels := doc.Find("#tab-releases > section > h3")

	for i := range releaseLabels.Length() {
		releaseLabelText := strings.TrimSpace(releaseLabels.Eq(i).Text())

		dates := releaseLabels.Eq(i).Next().Children()

		for j := range dates.Length() {
			dateStr := strings.TrimSpace(dates.Eq(j).Find("div > h5").Text())
			if dateStr == "" {
				return nil, fmt.Errorf("date can't be empty")
			}

			countries := dates.Eq(j).Find("div > ul > li")

			for k := range countries.Length() {
				release := models.Release{MovieId: movieId, Date: dateStr, ReleaseType: releaseLabelText}

				release.Country = strings.TrimSpace(countries.Eq(k).Find("span > span > span.name").Text())
				if release.Country == "" {
					return nil, fmt.Errorf("country name can't be empty")
				}

				ageRating := strings.TrimSpace(countries.Eq(k).Find("span > span > span > span.label").Text())
				if ageRating != "" {
					release.AgeRating = &ageRating
				}

				releases = append(releases, release)
			}
		}
	}

	return releases, nil
}
