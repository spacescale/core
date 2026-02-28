package main

import (
	"log/slog"
	"os"
)

const defaultGRPCAddress = ":9090"

// main boots the gRPC server and blocks serving requests.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

}
