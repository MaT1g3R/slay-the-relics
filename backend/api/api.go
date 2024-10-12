package api

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/MaT1g3R/slaytherelics/broadcaster"
	"github.com/MaT1g3R/slaytherelics/models"
	"github.com/MaT1g3R/slaytherelics/o11y"
)

// UserAuthenticator interface for user authentication implementations. See `slaytherelics` package for main implementation.
// TODO: Once we have Redis in the pipeline, we can get rid of this and use a stub for the twitch authentication only.
type UserAuthenticator interface {
	UserAuth(ctx context.Context, login, secret string) (bool, error)
	GetUserID(ctx context.Context, login string) (string, error)
	AuthenticateTwitch(ctx context.Context, code string) (models.User, string, error)
	AuthenticateRedis(ctx context.Context, userID, token string) (models.User, error)
}

type API struct {
	Router *gin.Engine

	users       UserAuthenticator
	broadcaster *broadcaster.Broadcaster

	// Increment to consider when reading msg.Delay. IE. is the caller sending us delay values in ms, s, etc.
	broadcastDelayIncrement time.Duration

	deckLists map[string]string
	deckLock  *sync.RWMutex
}

func New(u UserAuthenticator, b *broadcaster.Broadcaster) (*API, error) {
	r := gin.Default()
	r.Use(o11y.Middleware)

	err := r.SetTrustedProxies(nil)
	if err != nil {
		return nil, err
	}

	api := &API{
		Router: r,

		users:                   u,
		broadcaster:             b,
		broadcastDelayIncrement: time.Millisecond,

		deckLists: make(map[string]string),
		deckLock:  &sync.RWMutex{},
	}

	r.POST("/", api.postOldMessageHandler)
	r.POST("/api/v1/auth", api.Auth)
	r.POST("/api/v1/message", api.postMessageHandler)
	r.GET("/deck/:name", api.getDeckHandler)
	return api, nil
}
