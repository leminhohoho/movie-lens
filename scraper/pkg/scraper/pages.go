package scraper

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
	"github.com/leminhohoho/movie-lens/scraper/pkg/models"
	"github.com/leminhohoho/movie-lens/scraper/pkg/utils"
)

func ScrapeMemberPages(ctx context.Context, logger *slog.Logger) ([]models.User, error) {
	maxPage, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MAX_PAGE")))
	if err != nil {
		return nil, err
	}

	users := []models.User{}

	for i := range maxPage {
		var doc *goquery.Document

		if err := chromedp.Run(ctx,
			utils.NavigateTillTrigger(prefix+fmt.Sprintf("/members/popular/page/%d/", i+1),
				chromedp.WaitVisible("#content > div > div > section > table > tbody > tr:last-child"),
				utils.Delay(time.Second*2, time.Millisecond*300),
			),
			utils.ToGoqueryDoc("html", &doc),
		); err != nil {
			return nil, err
		}

		currentPageUsers, err := ExtractUsers(doc.Selection, logger)
		if err != nil {
			return nil, err
		}

		users = append(users, currentPageUsers...)
	}

	return users, nil
}
