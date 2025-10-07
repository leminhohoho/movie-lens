package scraper

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/chromedp/chromedp"
)

func createBaseCtx() (context.Context, context.CancelFunc) {
	opts := []func(*chromedp.ExecAllocator){
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", true),
		chromedp.UserDataDir(os.Getenv("USER_DATA_DIR")),
	}

	baseCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	return chromedp.NewContext(baseCtx)
}

func TestScrapeMemberPages(t *testing.T) {
	if os.Getenv("MAX_PAGE") == "" {
		os.Setenv("MAX_PAGE", "10")
	}

	if os.Getenv("USER_DATA_DIR") == "" {
		os.Setenv("USER_DATA_DIR", "/tmp/chromium-data")
	}

	ctx, cancel := createBaseCtx()
	defer cancel()

	logger := slog.Default()

	users, err := ScrapeMemberPages(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}

	for _, user := range users {
		logger.Info("user info", "user", user)
	}
}
