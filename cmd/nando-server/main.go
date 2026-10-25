// Command nando-server runs Nando's HTTP API.
//
//	nando-server          serve the API
//	nando-server enroll   print an Enrollment Code for registering a passkey
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	mongoURI string // MONGO_URI, required
	// DOMAIN, required: the parent domain of web. and api., such as
	// "nanders.app", which passkeys and the session cookie belong to.
	domain string
	// ALLOWED_ORIGINS, required: comma-separated web app origins, which may
	// call the API with credentials and use passkeys.
	allowedOrigins []string
	port           string // PORT, set by Cloud Run; defaults to 8080
}

func loadConfig() (config, error) {
	cfg := config{
		mongoURI: os.Getenv("MONGO_URI"),
		domain:   os.Getenv("DOMAIN"),
		port:     os.Getenv("PORT"),
	}
	if cfg.mongoURI == "" {
		return config{}, errors.New("MONGO_URI is required")
	}
	if cfg.domain == "" {
		return config{}, errors.New("DOMAIN is required")
	}
	if cfg.port == "" {
		cfg.port = "8080"
	}
	for origin := range strings.SplitSeq(os.Getenv("ALLOWED_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			if err := checkOrigin(origin); err != nil {
				return config{}, err
			}
			cfg.allowedOrigins = append(cfg.allowedOrigins, origin)
		}
	}
	if len(cfg.allowedOrigins) == 0 {
		return config{}, errors.New("ALLOWED_ORIGINS is required")
	}
	return cfg, nil
}

// checkOrigin rejects anything a browser would never send as an Origin, such as
// a trailing slash or "*", which would otherwise silently turn CORS off.
func checkOrigin(origin string) error {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("ALLOWED_ORIGINS: %q is not an origin like https://web.example.com", origin)
	}
	return nil
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
	defer func() {
		// Cloud Run allows 10s after SIGTERM: up to 8s draining requests, 2s here.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := dbs.Disconnect(ctx); err != nil {
			log.Printf("disconnect MongoDB: %v", err)
		}
	}()
	if err := c.Fill(&dbs); err != nil {
		return err
	}

	// Core and each Module get only their own database; Modules also get Core's
	// service. Calories' routes arrive with #10.
	nandoCore, err := core.New(connectCtx, dbs.Core, core.Config{
		Domain:  cfg.domain,
		Origins: cfg.allowedOrigins,
	})
	if err != nil {
		return err
	}
	_ = calories.NewCaloriesModule(dbs.Calories, nandoCore.Service())

	switch args := os.Args[1:]; {
	case len(args) == 1 && args[0] == "enroll":
		code, err := nandoCore.IssueEnrollmentCode(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Enrollment Code: %s\nValid for one passkey, for the next 15 minutes.\n", code)
		return nil
	case len(args) > 0:
		return fmt.Errorf("unknown arguments %q; the only command is enroll", args)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           server.New(server.Config{AllowedOrigins: cfg.allowedOrigins, Core: nandoCore}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		shutdownDone <- srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on :%s", cfg.port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// ListenAndServe returns as soon as Shutdown starts; wait for in-flight
	// requests to finish before MongoDB is disconnected.
	return <-shutdownDone
}
