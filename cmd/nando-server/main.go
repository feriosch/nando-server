// Command nando-server runs Nando's HTTP API.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golobby/container/v3"

	"github.com/feriosch/nando-server/internal/calories"
	"github.com/feriosch/nando-server/internal/core"
	"github.com/feriosch/nando-server/internal/server"
)

// config is read from the environment.
type config struct {
	mongoURI       string   // MONGO_URI, required
	allowedOrigins []string // ALLOWED_ORIGINS, comma-separated web app origins
	port           string   // PORT, set by Cloud Run; defaults to 8080
}

func loadConfig() (config, error) {
	cfg := config{
		mongoURI: os.Getenv("MONGO_URI"),
		port:     os.Getenv("PORT"),
	}
	if cfg.mongoURI == "" {
		return config{}, errors.New("MONGO_URI is required")
	}
	if cfg.port == "" {
		cfg.port = "8080"
	}
	for origin := range strings.SplitSeq(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			cfg.allowedOrigins = append(cfg.allowedOrigins, origin)
		}
	}
	return cfg, nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := container.New()
	if err := server.RegisterDatabases(connectCtx, c, cfg.mongoURI); err != nil {
		return err
	}
	var dbs server.Databases
	if err := c.Fill(&dbs); err != nil {
		return err
	}
	defer dbs.Disconnect(context.Background())

	// Core and each Module get only their own database; Modules also get Core's
	// service. Routes arrive with #7 and #10.
	nandoCore := core.NewCore(dbs.Core)
	_ = calories.NewCaloriesModule(dbs.Calories, nandoCore.Service())

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           server.New(server.Config{AllowedOrigins: cfg.allowedOrigins}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on :%s", cfg.port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
