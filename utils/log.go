package utils

import (
	"log/slog"
	"os"
)

var logLevel = new(slog.LevelVar)

func InitLogger(debug bool) {
	if debug {
		logLevel.Set(slog.LevelDebug)
	} else {
		logLevel.Set(slog.LevelInfo)
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Default level log.Printf→slog bridge
	_ = slog.SetLogLoggerLevel(debugValueToLevel(debug))
}

func debugValueToLevel(debug bool) slog.Level {
	if debug {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
