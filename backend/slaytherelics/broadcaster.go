package slaytherelics

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/instrument"

	errors2 "github.com/MaT1g3R/slaytherelics/errors"
	"github.com/MaT1g3R/slaytherelics/o11y"
)

const keepAlive = 5

var sentinel = struct{}{}

type PubSub interface {
	SendMessage(ctx context.Context, broadcasterID string, messageType int, message map[string]any) error
}

type Broadcaster struct {
	messages PubSub

	maxQueueSize      int
	keepAliveInterval time.Duration
	keepAliveTimeout  time.Duration

	senders sync.Map // map[string]*Sender

	sendersCounter instrument.Int64UpDownCounter
}

func NewBroadcaster(
	m PubSub, maxQueueSize int, keepAliveInterval, keepAliveTimeout time.Duration) (*Broadcaster, error) {
	sendersCounter, err := o11y.Meter.Int64UpDownCounter("broadcaster.senders.count")
	if err != nil {
		return nil, err
	}
	b := &Broadcaster{
		messages:          m,
		maxQueueSize:      maxQueueSize,
		keepAliveInterval: keepAliveInterval,
		keepAliveTimeout:  keepAliveTimeout,
		sendersCounter:    sendersCounter,
	}
	return b, nil
}

func (b *Broadcaster) Broadcast(ctx context.Context,
	delay time.Duration, broadcasterID string, messageType int, message map[string]any) (err error) {
	ctx, span := o11y.Tracer.Start(ctx, "broadcaster: broadcast")
	defer o11y.End(&span, &err)

	s, ok := b.senders.LoadOrStore(broadcasterID, newSender(broadcasterID, b))
	span.SetAttributes(attribute.Bool("cache_hit", ok))

	sender := s.(*sender)
	if !ok {
		b.sendersCounter.Add(ctx, 1, attribute.String("broadcaster_id", broadcasterID))
		sender.init(ctx)
	}

	time.Sleep(delay)
	return sender.send(ctx, messageType, message)
}

type sender struct {
	// message type -> latest message
	state sync.Map

	broadcasterID string

	broadcaster *Broadcaster

	queue chan struct{}

	terminated atomic.Bool
}

func newSender(id string, b *Broadcaster) *sender {
	return &sender{
		state:         sync.Map{},
		broadcasterID: id,
		broadcaster:   b,
		terminated:    atomic.Bool{},
	}
}

func (s *sender) init(ctx context.Context) {
	ctx, span := o11y.Tracer.Start(ctx, "broadcaster: init keep alive worker")
	defer o11y.End(&span, nil)

	span.SetAttributes(
		attribute.String("broadcaster_id", s.broadcasterID),
		attribute.Int("max_queue_size", s.broadcaster.maxQueueSize),
		attribute.Int64("keep_alive_interval_ms", s.broadcaster.keepAliveInterval.Milliseconds()),
		attribute.Float64("keep_alive_timeout_s", s.broadcaster.keepAliveTimeout.Seconds()),
	)

	s.queue = make(chan struct{}, s.broadcaster.maxQueueSize)
	go s.keepAliveWorker(ctx)
}

func (s *sender) terminate(ctx context.Context) {
	s.terminated.Store(true)
	s.broadcaster.senders.Delete(s.broadcasterID)
	s.broadcaster.sendersCounter.Add(ctx, -1, attribute.String("broadcaster_id", s.broadcasterID))
}

func (s *sender) keepAliveWorker(ctx context.Context) {
	ctx, span := o11y.Tracer.Start(o11y.Detach(ctx), "broadcaster: keep alive worker")
	defer o11y.End(&span, nil)

	for {
		if s.terminated.Load() {
			return
		}

		time.Sleep(s.broadcaster.keepAliveInterval)

		select {
		case <-s.queue:
			_ = s.sendAll()
			continue
		case <-time.After(s.broadcaster.keepAliveTimeout):
			span.AddEvent("keepalive worker killed")
			s.terminate(ctx)
			return
		}
	}
}

func (s *sender) sendAll() (err error) {
	// ctx is deliberately background context to have the span be standalone
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	ctx, span := o11y.Tracer.Start(ctx, "broadcaster: send all")
	defer o11y.End(&span, &err)

	span.SetAttributes(attribute.String("broadcaster_id", s.broadcasterID))

	s.state.Range(func(typ, msg interface{}) bool {
		sErr := s.broadcaster.messages.SendMessage(ctx, s.broadcasterID, typ.(int), msg.(map[string]any))
		err = errors.Join(err, sErr)
		return true
	})
	return err
}

func (s *sender) send(ctx context.Context, typ int, m map[string]any) (err error) {
	ctx, span := o11y.Tracer.Start(ctx, "broadcaster: send message")
	defer o11y.End(&span, &err)
	span.SetAttributes(attribute.Bool("keep_alive", typ == keepAlive))

	if typ != keepAlive {
		s.state.Store(typ, m)
		return s.broadcaster.messages.SendMessage(ctx, s.broadcasterID, typ, m)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*10)
	defer cancel()

	select {
	case s.queue <- sentinel:
		return nil
	case <-ctx.Done():
		return &errors2.Timeout{Err: ctx.Err()}
	}
}
