package logger

import (
	"io"
	"log/slog"
	"os"
)

func NewLogger() (*slog.Logger, error) {
	opts := &slog.HandlerOptions{}
	var output io.Writer

	if os.Getenv("DEBUG") == "TRUE" {
		opts.Level = slog.LevelDebug
	}

	if os.Getenv("SILENT") == "TRUE" {
		logFilePath := os.Getenv("LOG_FILE_PATH")
		if logFilePath == "" {
			logFilePath = "/tmp/movie_lens.log"
		}

		var err error
		output, err = os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
	} else {
		output = os.Stdout
	}

	return slog.New(slog.NewJSONHandler(output, opts)), nil
}
