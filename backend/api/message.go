package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	errors2 "github.com/MaT1g3R/slaytherelics/errors"
	"github.com/MaT1g3R/slaytherelics/o11y"
	"github.com/MaT1g3R/slaytherelics/slaytherelics"
)

func (a *API) postMessageHandler(c *gin.Context) {
	var err error
	ctx, span := o11y.Tracer.Start(c.Request.Context(), "api: post message")
	defer o11y.End(&span, &err)

	pubSubMessage := slaytherelics.PubSubMessage{}
	err = c.BindJSON(&pubSubMessage)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// do some initial validation on the parsed message
	if pubSubMessage.Streamer == nil || pubSubMessage.Streamer.Login == "" || pubSubMessage.Streamer.Secret == "" {
		err := &errors2.AuthError{Err: errors.New("missing login or secret")}
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// make sure that we have message content len > 0
	if pubSubMessage.MessageContent == nil {
		c.JSON(400, gin.H{})
		return
	}

	user, err := a.users.AuthenticateRedis(ctx, pubSubMessage.Streamer.Login, pubSubMessage.Streamer.Secret)
	authError := &errors2.AuthError{}
	if errors.As(err, &authError) {
		c.JSON(401, gin.H{"error": authError.Error()})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if pubSubMessage.MessageType == slaytherelics.MessageTypeDeck {
		func() {
			a.deckLock.Lock()
			defer a.deckLock.Unlock()

			content := pubSubMessage.MessageContent.(slaytherelics.MessageContentDeck)

			a.deckLists[strings.ToLower(user.Login)] = content.Deck
		}()
	}
	err = a.broadcast(c, ctx, pubSubMessage, user.ID)
}

func (a *API) broadcast(c *gin.Context,
	ctx context.Context, msg slaytherelics.PubSubMessage, userID string) error {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("broadcast", trace.WithAttributes(
		attribute.String("streamer", userID),
		attribute.Int("message_type", int(msg.MessageType)),
		attribute.Int("delay_ms", msg.Delay),
	))

	err := a.broadcaster.Broadcast(ctx, time.Duration(msg.Delay)*a.broadcastDelayIncrement, userID, int(msg.MessageType), msg.MessageContent)
	timeout := &errors2.Timeout{}
	if errors.As(err, &timeout) {
		c.Data(202, "application/json; charset=utf-8", []byte("Success\n"))
		return err
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return err
	}
	c.Data(200, "application/json; charset=utf-8", []byte("Success\n"))
	return nil
}
