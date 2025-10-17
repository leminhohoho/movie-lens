package utils

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

// Delay return a chromedp.Action that will pause a random amount of time based on the base and deviation.
// The pause duration is in the range of [base - deviation, base + deviation]
func Delay(base time.Duration, deviation time.Duration) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		if base < deviation {
			return fmt.Errorf("base (%d) is smaller than deviation (%d)", int(base), int(deviation))
		}

		time.Sleep(base + time.Duration(rand.IntN(int(deviation)*2)-int(deviation)))

		return nil
	}
}

// ToGoqueryDoc is a wrapper for chromedp.OuterHTML.
// It parses *goquery.Document to the underlying value of the pointer.
func ToGoqueryDoc(sel string, doc **goquery.Document) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var html string

		if err := chromedp.OuterHTML(sel, &html).Do(ctx); err != nil {
			return err
		}

		d, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			return err
		}

		*doc = d

		return nil
	}
}

// NavigateTillTrigger run chromedp.Navigate() and a list of chromedp.Action simultaneously (the actions are ran sequentially).
// If the actions are all finished, the function will close.
// If no action is provided, it will run chromedp.Navigate concurrently
func NavigateTillTrigger(actionToTrigger chromedp.Action, logger *slog.Logger, actions ...chromedp.Action) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		navErrChan := make(chan error, 1)
		actErrChan := make(chan error, 1)

		logger.Debug("start navigation", "tags", []string{"helper"})
		go func() { navErrChan <- ActionWithRetries(3, actionToTrigger).Do(ctx) }()
		go func() {
			for i, action := range actions {
				err := action.Do(ctx)
				if err != nil {
					actErrChan <- err
				}

				logger.Debug("action finished", "order", i+1, "tags", []string{"helper"})
			}

			actErrChan <- nil
		}()

		for {
			select {
			case err := <-navErrChan:
				if err != nil {
					return err
				}
			case err := <-actErrChan:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// ActionWithRetries run an action that triggers http request and retry it if the request failed.
// The action must invoke HTTP request, otherwise it will be blocked until the context is cancelled.
func ActionWithRetries(retries int, action chromedp.Action) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		for i := range retries {
			res, err := chromedp.RunResponse(ctx, action)
			if err != nil {
				return err
			}

			switch int(res.Status) {
			case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
				advance := 30000 * (i + 1) / 3
				time.Sleep(time.Second*30 + time.Millisecond*time.Duration(advance))
				continue
			case http.StatusOK:
				return nil
			}
		}

		return nil
	}
}
