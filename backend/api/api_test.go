package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gotest.tools/v3/assert"

	"github.com/MaT1g3R/slaytherelics/broadcaster"
	errors2 "github.com/MaT1g3R/slaytherelics/errors"
	"github.com/MaT1g3R/slaytherelics/models"
	"github.com/MaT1g3R/slaytherelics/o11y"
	"github.com/MaT1g3R/slaytherelics/slaytherelics"
)

// dummyMessage SendMessage param storage
type dummyMessage struct {
	broadcasterID string
	typ           int
	message       string
	messageIface  interface{}
}

// pubSubStub stub implementation of PubSub interface.
type pubSubStub struct {
	messageChan chan dummyMessage
}

// SendMessage stub implementation of PubSub.SendMessage.
func (p *pubSubStub) SendMessage(ctx context.Context,
	broadcasterID string, messageType int, message interface{}) error {

	js, err := json.Marshal(message)
	if err != nil {
		return err
	}

	msg := dummyMessage{
		broadcasterID: broadcasterID,
		typ:           messageType,
		message:       string(js),
		messageIface:  message,
	}

	select {
	case p.messageChan <- msg:
	default: // if no one is listening, don't block
	}

	return nil
}

// userAuthStub stub implementation of UserAuth.
// TODO: remove this once Redis is in CI
type userAuthStub struct {
	redisUserMap map[string]models.User
}

// TODO: AuthenticateTwitch, twitch endpoint
func (u *userAuthStub) AuthenticateTwitch(ctx context.Context, code string) (models.User, string, error) {
	return models.User{}, "token", nil
}

// TODO: UserAuth, twitch endpoint
func (u *userAuthStub) UserAuth(ctx context.Context, login, secret string) (bool, error) {
	return true, nil
}

// TODO: GetUserID, twitch endpoint
func (u *userAuthStub) GetUserID(ctx context.Context, login string) (string, error) {
	return "userID", nil
}

// AuthenticateRedis stub implementation of AuthenticateRedis.
func (u *userAuthStub) AuthenticateRedis(ctx context.Context, userID, token string) (models.User, error) {
	usr, ok := u.redisUserMap[userID]
	if !ok {
		return models.User{}, &errors2.AuthError{Err: errors.New("user not found")}
	}

	if usr.Hash != token {
		return models.User{}, &errors2.AuthError{Err: errors.New("invalid token")}
	}

	return models.User{}, nil
}

func TestAPIHandler(t *testing.T) {
	ctx := context.Background()
	cancel := o11y.Init("test")
	defer cancel(ctx)

	authStub := &userAuthStub{
		redisUserMap: map[string]models.User{
			"arrakisdev": {
				ID:    "arrakisdev",
				Hash:  "yxk1d4kv3zz5bb4u8byp",
				Login: "arrakisdev",
			},
		},
	}

	pubSubStub := &pubSubStub{
		messageChan: make(chan dummyMessage, 1), // buffer 1 so that we can read even if we start the read after the write
	}

	bc, err := broadcaster.NewBroadcaster(pubSubStub, 2, time.Second*1, time.Second*2)
	assert.NilError(t, err)

	// TODO: stub for twitch service
	apiHandler, err := New(authStub, bc)
	assert.NilError(t, err)

	apiHandler.broadcastDelayIncrement = time.Nanosecond

	handlerFunc := apiHandler.Router.Handler()

	t.Run("getDeckHandler", func(t *testing.T) {
		type testCase struct {
			name           string
			deckRawContent string
			expectedStatus int
			expectedOutput string
		}

		tcs := []testCase{
			{
				name:           "Empty",
				expectedStatus: 500, // error parsing deck
			},
			{
				name:           "Invalid card index",
				deckRawContent: "card|junk||};;;&01;&1;x;;&02;&1;y;;&03;&1;z",
				expectedStatus: 500,
			},
			{
				name:           "Negative card index",
				deckRawContent: "card|junk||-1;;;&01;&1;x;;&02;&1;y;;&03;&1;z",
				expectedStatus: 500,
			},
			{
				name:           "Out of bounds card index",
				deckRawContent: "card|junk||3;;;&01;&1;x;;&02;&1;y;;&03;&1;z",
				expectedStatus: 500,
			},
			{
				name:           "Simple deck",
				deckRawContent: "card|junk||0,1,1,0,2,0;;;&01;&1;x;;&02;&1;y;;&03;&1;z",
				expectedOutput: "card1 x3\ncard2 x2\ncard3 x1\n",
				expectedStatus: 200,
			},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				deckName := uuid.New().String()
				// use lock if adding t.Parallel
				apiHandler.deckLists[deckName] = tc.deckRawContent

				req := httptest.NewRequest("GET", "/deck/"+deckName, nil)
				w := httptest.NewRecorder()
				handlerFunc.ServeHTTP(w, req)

				assert.Equal(t, w.Code, tc.expectedStatus)

				if tc.expectedOutput != "" {
					assert.Equal(t, w.Body.String(), tc.expectedOutput, w.Body.String(), tc.expectedOutput)
				}
			})
		}

		t.Run("not found", func(t *testing.T) {
			req := httptest.NewRequest("GET", "/deck/"+uuid.New().String(), nil)
			w := httptest.NewRecorder()
			handlerFunc.ServeHTTP(w, req)

			assert.Equal(t, w.Code, 404)
		})
	})

	t.Run("postMessageHandler", func(t *testing.T) {
		type testCase struct {
			name          string
			inputContent  string
			inputFilePath string

			expectedStatus           int
			expectedPublishedMessage interface{}
		}

		tcs := []testCase{
			{
				name:           "Empty",
				inputContent:   "{}",
				expectedStatus: 400,
			},
			{
				name:           "Invalid",
				inputContent:   "{ \"some\", invalid JSON",
				expectedStatus: 400,
			},
			{
				name:           "Unauthorized user not found",
				inputContent:   `{ "streamer": { "login": "invalid", "secret": "invalid" }, "message": {} }`,
				expectedStatus: 401,
			},
			{
				name:           "Unauthorized invalid password",
				inputContent:   `{ "streamer": { "login": "arrakisdev", "secret": "invalid" }, "message": {} }`,
				expectedStatus: 401,
			},
			{
				name:           "Deck msg type",
				inputFilePath:  "../slaytherelics/testdata/deck_msg.json", // Mod ref - https://github.com/Spireblight/STR-Extension/blob/c069792cdda4c7a01266cc41f2ea8b1583162d4b/twitch_demo/deck_msg.txt
				expectedStatus: 200,
				expectedPublishedMessage: slaytherelics.MessageContentDeck{
					Character: "Defect",
					Deck:      "your|damage|turn|twice|.;;|next|first|#yLightning|played|#yChannel|Orb|#yEvoke|ability|trigger|the|#yBlock|passive|The &6 card you play NL each &2 is &8|&e start of &0 NL &2, &d &e NL|;0;-;-1;-1;+;0;-;|At &i &g &c of &0 NL &5|;0;-;-1;-1;+;0;|;-;-1;-1;_;1;-;|;&b &0 &5 &a NL |;1;0;1;0;| &e end of |.;-;1;1;|,1,1,1,| cards are |;1;1;|;2;| this | NL | &a | &0 | empty ||0,0,0,0&r1,2,3,4,5,6,7;;;Strike&jDeal #g9 &1&q0;0&vDeal 6 &1&4Defend&l0;Gain #g8 &f.;0&t1;0&vGain 5 &f&4Zap&l1,2;&9 1 &7.;1,2&o2;&9 1 &7&4Dualcast&l3&n&3.;3&o2&n&3&4Echo Form&j&h&x&3.;4;3;3&v4&v#yE&ereal.&x&h&x&3&4Streamline+;0&m_;-&v2;0&v2;Deal 20 &1.&xReduce&wcard's cost&xby 1&wcombat&4Amplify+;0&m_;-&t1;4&vThis &2,&z&5 2&xPower&s&8&x&3&4Loop&j&k&y#g2 times&q2;3&v&k &a&4;Block;Until &5 &2, prevents &1&4Channel;Channeling an&yputs it into&z&6&Aslot. If you have no&Aslots,&z&6&yis automatically &bd to make room&4Lightning;&a: Deals &1 to random enemies&4Evoke;Consume&zrightmost&yand use its Evoke effect&4E&ereal;If&wcard is in&zhand at&p&2, it is exhausted. Exhausted&sremoved from&zdeck until&pcombat.",
				},
			},
			{
				name:           "Okay msg JSON",
				inputFilePath:  "../slaytherelics/testdata/okay_msg.json", // Mod ref - https://github.com/Spireblight/STR-Extension/blob/c069792cdda4c7a01266cc41f2ea8b1583162d4b/twitch_demo/okay_msg.txt
				expectedStatus: 200,
				// should not publish msg if keepalive
			},
			{
				// should still parse even though we have not implemented the type yet
				name:                     "Tips msg JSON",
				inputFilePath:            "../slaytherelics/testdata/tips_msg.json", // Mod ref - https://github.com/Spireblight/STR-Extension/blob/c069792cdda4c7a01266cc41f2ea8b1583162d4b/twitch_demo/tips_msg.txt
				expectedStatus:           200,
				expectedPublishedMessage: slaytherelics.MessageContentUnknown(`{"c":"Defect","r":[0,[[0,1,2],[3],[4,5],[6],[7]]],"o":[667,[[8,9],[10,11],[12]]],"p":[18.75,46.39,12.5,29.26,[]],"m":[[68.23,52.78,13.54,22.41,[13]]],"u":[[31.67,46.19,5,8.89,[14]],[22.5,35.19,5,8.89,[15]],[13.33,46.19,5,8.89,[16]]],"w":"damage|Dexterity|.;;|#yPassive|combat|#yEvoke|#yBlock|enemy|first|Potion|random|Lightning|turn|the|Focus|enemies|;&3: At &d end of &c, | &0 to a &a |. NL &5: Deal | &9;Gain #b2 | each &4 | your &8 | &0 to ||Cracked Core;At &d start of each &4, #yChannel #b1 #y&b&2Channel;Channeling an Orb puts it into&lempty slot. If you have no empty slots,&lOrb is automatically &5d to make room&2&b;Orb: Deals&m&a &f&2Neow's Lament;Enemies in&l#b3 &4s will have #b1 HP&2Anchor;Start&kwith #b10 &6&2Block;Until next &c, prevents &0&2Akabeko;Your &8 Attack&kdeals #b8 additional &0&2Omamori;Negate &d next #b2 #rCurses you obtain&2&1&j#y&1&2&1;&1 improves Block gained from cards&2&e&j#y&e&2&e;&e increases &d effectiveness of #yChanneled Orbs&2&9 Slot;Use potions during &4 to gain #gbonuses or #rhinder &f&2Aggressive;This &7 intends to NL #yAttack for #b12 &0.;intents/tip/3;;&b&gdeal #b3&h&7&i#b8&h&7&2Dark&gincrease this Orb's &0 by #b6&i#b6&m&d &7 with lowest HP&2Frost&ggain #b2 &6. NL &5: Gain #b5 &6."}`),
			},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				var err error
				var inputBody []byte // for later use in test
				if tc.inputFilePath != "" {
					inputBody, err = os.ReadFile(tc.inputFilePath)
					assert.NilError(t, err)
				} else {
					inputBody = []byte(tc.inputContent)
				}

				// drain channel
				select {
				case <-pubSubStub.messageChan:
				default:
				}

				req := httptest.NewRequest("POST", "/api/v1/message", bytes.NewReader(inputBody))
				w := httptest.NewRecorder()
				handlerFunc.ServeHTTP(w, req) // blocks until req is done, so we should expect a message in the channel by the time it returns

				assert.Equal(t, w.Code, tc.expectedStatus)

				if tc.expectedPublishedMessage != nil {
					var msg dummyMessage
					select {
					case msg = <-pubSubStub.messageChan:
					case <-time.After(time.Second):
						t.Fatalf("timeout waiting for message to be sent")
					}

					assert.DeepEqual(t, msg.messageIface, tc.expectedPublishedMessage)

					// make sure that we match the same output as when processing with the old path, which was
					// unmarshalling the msg as map[string]interface{} and then marshalling it again at send time

					var oldMsg map[string]interface{}
					err = json.Unmarshal(inputBody, &oldMsg)
					assert.NilError(t, err)

					oldMsgContent, ok := oldMsg["message"]
					assert.Assert(t, ok)

					oldMsgContentMapIface, ok := oldMsgContent.(map[string]interface{})
					assert.Assert(t, ok)

					// since to order of the keys is not really deterministic, we can't just compare the bytes
					// compare sections of each str that should be present at some point
					for k, v := range oldMsgContentMapIface {
						// should have single instance of key
						kc := strings.Count(msg.message, fmt.Sprintf(`"%s"`, k))
						assert.Equal(t, kc, 1)

						// should have single instance of value
						vBts, err := json.Marshal(v)
						assert.NilError(t, err)

						vc := strings.Count(msg.message, string(vBts))
						assert.Equal(t, vc, 1)
					}
				} else {
					select {
					case <-pubSubStub.messageChan:
						t.Fatalf("unexpected message sent")
					case <-time.After(time.Millisecond * 50):
					}
				}
			})
		}
	})
}
