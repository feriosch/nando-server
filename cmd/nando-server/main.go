// Command nando-server runs Nando's HTTP API.
//
//	nando-server          serve the API
//	nando-server enroll   print an Enrollment Code for registering a Passkey
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
	"github.com/feriosch/nando-server/internal/japanese"
	"github.com/feriosch/nando-server/internal/server"
)

// config is read from the environment.
type config struct {
	mongoURI string // MONGO_URI, required
	// DOMAIN, required: the parent domain of web. and api., such as
	// "nanders.app", which Passkeys and the Session cookie belong to.
	domain string
	// ALLOWED_ORIGINS, required: comma-separated web app origins, which may
	// call the API with credentials and use Passkeys.
	allowedOrigins []string
	port           string // PORT, set by Cloud Run; defaults to 8080

	// VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY and PUSH_CONTACT, required to
	// serve: the key pair (base64url) and the mailto: or https: contact that
	// sign push messages.
	vapid core.VAPID
	// TICK_AUDIENCE and SCHEDULER_SERVICE_ACCOUNT, required to serve:
	// /internal/tick accepts only Google-signed OIDC tokens for this audience
	// (as the Cloud Scheduler job sets it) issued to this service account.
	tickAudience, schedulerServiceAccount string
	// CLAUDE_TRIGGER_URL and CLAUDE_TRIGGER_TOKEN, optional: the API trigger
	// of the cloud scheduled task that works through Word Requests, and its
	// bearer token. Without both, Claude is never woken.
	claudeTriggerURL, claudeTriggerToken string
}

func loadConfig() (config, error) {
	cfg := config{
		mongoURI: os.Getenv("MONGO_URI"),
		domain:   os.Getenv("DOMAIN"),
		port:     os.Getenv("PORT"),

		vapid: core.VAPID{
			PublicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
			PrivateKey: os.Getenv("VAPID_PRIVATE_KEY"),
			Contact:    os.Getenv("PUSH_CONTACT"),
		},
		tickAudience:            os.Getenv("TICK_AUDIENCE"),
		schedulerServiceAccount: os.Getenv("SCHEDULER_SERVICE_ACCOUNT"),
		claudeTriggerURL:        os.Getenv("CLAUDE_TRIGGER_URL"),
		claudeTriggerToken:      os.Getenv("CLAUDE_TRIGGER_TOKEN"),
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

// checkNotifications reports a missing Notifications setting. Only serving
// needs them, not `nando-server enroll`.
func (cfg config) checkNotifications() error {
	for _, v := range []struct{ name, value string }{
		{"VAPID_PUBLIC_KEY", cfg.vapid.PublicKey},
		{"VAPID_PRIVATE_KEY", cfg.vapid.PrivateKey},
		{"PUSH_CONTACT", cfg.vapid.Contact},
		{"TICK_AUDIENCE", cfg.tickAudience},
		{"SCHEDULER_SERVICE_ACCOUNT", cfg.schedulerServiceAccount},
	} {
		if v.value == "" {
			return fmt.Errorf("%s is required", v.name)
		}
	}
	return nil
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
	// service.
	nandoCore, err := core.New(connectCtx, dbs.Core, core.Config{
		Domain:  cfg.domain,
		Origins: cfg.allowedOrigins,

		VerifySchedulerToken: core.VerifyGoogleSchedulerToken(cfg.tickAudience, cfg.schedulerServiceAccount),
		VAPID:                cfg.vapid,
	})
	if err != nil {
		return err
	}
	caloriesModule, err := calories.NewCaloriesModule(connectCtx, dbs.Calories, nandoCore.Service(), calories.Config{})
	if err != nil {
		return err
	}
	nandoCore.AddNotificationProvider(caloriesModule.NotificationProvider())
	japaneseModule, err := japanese.NewJapaneseModule(connectCtx, dbs.Japanese, nandoCore.Service(), japanese.Config{
		ClaudeTriggerURL:   cfg.claudeTriggerURL,
		ClaudeTriggerToken: cfg.claudeTriggerToken,
	})
	if err != nil {
		return err
	}
	nandoCore.AddToolProvider(japaneseModule.ToolProvider())
	nandoCore.AddTickJob(japaneseModule.TickJob())

	switch args := os.Args[1:]; {
	case len(args) == 1 && args[0] == "enroll":
		code, err := nandoCore.IssueEnrollmentCode(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("Enrollment Code: %s\nValid for one Passkey, for the next 15 minutes.\n", code)
		return nil
	case len(args) > 0:
		return fmt.Errorf("unknown arguments %q; the only command is enroll", args)
	}
	if err := cfg.checkNotifications(); err != nil {
		return err
	}

	srv := &http.Server{
		Addr: ":" + cfg.port,
		Handler: server.New(server.Config{
			AllowedOrigins: cfg.allowedOrigins,
			Core:           nandoCore,
			Modules:        []server.Module{caloriesModule, japaneseModule},
		}),
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
