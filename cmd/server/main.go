package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/a-tho/monitor/internal/config"
	"github.com/a-tho/monitor/internal/server"
	"github.com/a-tho/monitor/internal/storage"
)

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	var (
		cfg config.Config
		err error
	)
	if err := cfg.ParseConfig(); err != nil {
		return err
	}
	cfg.InitLogger()

	cfg.Log()

	ctx := context.Background()
	cfg.Metrics, err = storage.New(ctx, cfg.DatabaseDSN, cfg.FileStoragePath, cfg.StoreInterval, cfg.Restore)
	if err != nil {
		return err
	}
	defer cfg.Metrics.Close()

	mux := server.NewServer(cfg.Metrics, cfg.Key)
	go func() {
		if err := http.ListenAndServe(cfg.SrvAddr, mux); err != nil {
			panic(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT)
	signal.Notify(quit, syscall.SIGQUIT)

	<-quit

	return nil
}
