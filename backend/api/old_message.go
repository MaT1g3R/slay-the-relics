package api

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"

	errors2 "github.com/MaT1g3R/slaytherelics/errors"
	"github.com/MaT1g3R/slaytherelics/o11y"
)

type RequestMessage struct {
	MessageType int `json:"msg_type"`
	Streamer    struct {
		Login  string `json:"login"`
		Secret string `json:"secret"`
	} `json:"streamer"`
	Metadata map[string]any `json:"meta"`
	Delay    int            `json:"delay"`
	Message  any            `json:"message"`
}

func (a *API) oldAuthenticate(c *gin.Context, ctx context.Context, login, secret string) (_ string, err error) {
	ctx, span := o11y.Tracer.Start(ctx, "api: oldAuthenticate")
	defer o11y.End(&span, &err)

	span.SetAttributes(attribute.String("login", login))

	if login == "" || secret == "" {
		err := &errors2.AuthError{Err: errors.New("missing login or secret")}
		c.JSON(400, gin.H{"error": err.Error()})
		return "", err
	}

	auth, err := a.users.UserAuth(ctx, login, secret)
	authError := &errors2.AuthError{}
	if errors.As(err, &authError) {
		c.JSON(401, gin.H{"error": authError.Error()})
		return "", err
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return "", err
	}
	if !auth {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return "", &errors2.AuthError{Err: errors.New("unauthorized")}
	}

	streamer, err := a.users.GetUserID(ctx, login)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return "", err
	}

	span.SetAttributes(attribute.String("user_id", streamer))
	return streamer, nil
}

func (a *API) postOldMessageHandler(c *gin.Context) {
	var err error
	ctx, span := o11y.Tracer.Start(c.Request.Context(), "api: old post message")
	defer o11y.End(&span, &err)

	req := RequestMessage{}
	err = c.BindJSON(&req)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	message := make(map[string]any)
	switch t := req.Message.(type) {
	case string:
		if len(t) != 0 {
			c.JSON(400, gin.H{})
			return
		}
	case map[string]any:
		message = t
	default:
		c.JSON(400, gin.H{})
		return
	}

	login := strings.ToLower(req.Streamer.Login)
	streamer, err := a.oldAuthenticate(c, ctx, login, req.Streamer.Secret)
	if err != nil {
		return
	}
	err = a.broadcast(c, ctx, req, streamer, message)
}
