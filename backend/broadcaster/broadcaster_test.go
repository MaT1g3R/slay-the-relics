package broadcaster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"gotest.tools/v3/assert"

	"github.com/MaT1g3R/slaytherelics/o11y"
)

type dummyMessage struct {
	broadcasterID string
	typ           int
	message       string
}

type pubSubStub struct {
	messages []dummyMessage
}

func (p *pubSubStub) SendMessage(ctx context.Context,
	broadcasterID string, messageType int, message interface{}) error {

	js, err := json.Marshal(message)
	if err != nil {
		return err
	}

	p.messages = append(p.messages, dummyMessage{
		broadcasterID: broadcasterID,
		typ:           messageType,
		message:       string(js),
	})
	return nil
}

func TestBroadcaster(t *testing.T) {
	ctx := context.Background()
	cancel := o11y.Init("test")
	defer cancel(ctx)

	pubsub := &pubSubStub{
		messages: []dummyMessage{},
	}
	broadcasterID1 := "broadcasterID1"
	broadcasterID2 := "broadcasterID2"

	messageKeepAlive := map[string]any{}
	messageOther := map[string]any{"a": "a"}
	messageOther2 := map[string]any{"a": "b"}

	broadcaster, err := NewBroadcaster(pubsub, 2, 10*time.Millisecond, time.Second*2)
	assert.NilError(t, err)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID1, keepAlive, messageKeepAlive)
	assert.NilError(t, err)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID2, keepAlive, messageKeepAlive)
	assert.NilError(t, err)

	time.Sleep(1 * time.Second)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID1, 2, messageOther)
	assert.NilError(t, err)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID2, 2, messageOther)
	assert.NilError(t, err)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID1, 3, messageOther2)
	assert.NilError(t, err)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID2, 3, messageOther2)
	assert.NilError(t, err)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID1, keepAlive, messageKeepAlive)
	assert.NilError(t, err)

	err = broadcaster.Broadcast(ctx, time.Nanosecond, broadcasterID2, keepAlive, messageKeepAlive)
	assert.NilError(t, err)

	time.Sleep(time.Second * 3)
	_, ok := broadcaster.senders.Load(broadcasterID1)
	assert.Check(t, !ok)
	_, ok = broadcaster.senders.Load(broadcasterID2)
	assert.Check(t, !ok)

	gotMessages := map[string][]string{}
	for _, val := range pubsub.messages {
		gotMessages[val.broadcasterID] = append(gotMessages[val.broadcasterID], val.message)
	}

	assert.DeepEqual(t, gotMessages[broadcasterID1], []string{`{"a":"a"}`, `{"a":"b"}`, `{"a":"a"}`, `{"a":"b"}`},
		cmpopts.SortSlices(func(a, b string) bool { return a < b }))
	assert.DeepEqual(t, gotMessages[broadcasterID2], []string{`{"a":"a"}`, `{"a":"b"}`, `{"a":"a"}`, `{"a":"b"}`},
		cmpopts.SortSlices(func(a, b string) bool { return a < b }))
}
