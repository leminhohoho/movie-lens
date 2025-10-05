package scraper

import (
	"fmt"
	"log/slog"

	"github.com/PuerkitoBio/goquery"
	"github.com/leminhohoho/movie-lens/scraper/pkg/models"
)

// ExtractUser get all users information from the member page at https://letterboxd.com/members/popular.
// It return a list of users and error if the extracting process fails.
func ExtractUser(doc *goquery.Selection, logger *slog.Logger) ([]models.User, error) {
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
