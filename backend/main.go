package main

import (
	"context"
	"net/http"
	_ "net/http/pprof"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/MaT1g3R/slaytherelics/api"
	"github.com/MaT1g3R/slaytherelics/broadcaster"
	"github.com/MaT1g3R/slaytherelics/client"
	"github.com/MaT1g3R/slaytherelics/config"
	"github.com/MaT1g3R/slaytherelics/o11y"
	"github.com/MaT1g3R/slaytherelics/slaytherelics"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	a, cancel, err := initialize(ctx, cfg)
	defer cancel(ctx)
	if err != nil {
		panic(err)
	}

	err = a.Router.Run(cfg.ListenAddr)
	if err != nil {
		panic(err)
	}
}

func initialize(ctx context.Context, cfg config.Config) (_ *api.API, cancel func(context.Context), err error) {
	cancel = o11y.Init("slay-the-relics")

	ctx, span := o11y.Tracer.Start(ctx, "init")
	defer o11y.End(&span, &err)

	span.AddEvent("initializing",
		trace.WithAttributes(attribute.String("listen-addr", cfg.ListenAddr)),
	)

	span.AddEvent("starting pprof listener")
	setupPprofListener(cfg)

	twitchClient, err := client.New(
		ctx,
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.OwnerUserID,
		cfg.ExtensionSecret,
	)
	if err != nil {
		return nil, cancel, err
	}

	rdb := client.NewRedis(cfg.RedisAddr)
	users := slaytherelics.NewUsers(twitchClient, rdb)
	messages := slaytherelics.NewMessages(twitchClient)
	broadcaster, err := broadcaster.NewBroadcaster(messages, 20, time.Second*3, time.Minute)
	if err != nil {
		return nil, cancel, err
	}

	span.AddEvent("starting server")
	a, err := api.New(users, broadcaster)
	return a, cancel, err
}

// setupPprofListener runs a background listener for handling pprof requests. This is done separately from any other router
// to avoid middleware conflicts or auth confusion. This also allows us to easily expose this port locally for our debugging
// but not to the wider web.
func setupPprofListener(cfg config.Config) {
	go func() {
		http.ListenAndServe(cfg.PprofAddr, nil)
	}()
}
