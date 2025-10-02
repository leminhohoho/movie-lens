package logger

import (
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
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

		output = &lumberjack.Logger{
			Filename:   logFilePath,
			MaxSize:    500,
			MaxAge:     30,
			MaxBackups: 3,
			Compress:   true,
			LocalTime:  true,
		}
	} else {
		output = os.Stdout
	}

	return slog.New(slog.NewJSONHandler(output, opts)), nil
}
