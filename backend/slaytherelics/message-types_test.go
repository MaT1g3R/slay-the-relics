package slaytherelics

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestPubSubMessageUnmarshalJSON(t *testing.T) {
	type testCase struct {
		name           string
		inputFilePath  string
		inputStr       string
		expectedErr    bool
		expectedOutput PubSubMessage
	}

	tcs := []testCase{
		{
			name:           "Empty",
			inputStr:       "{}",
			expectedOutput: PubSubMessage{},
		},
		{
			name:        "Invalid",
			inputStr:    "{ \"some\", invalid JSON",
			expectedErr: true,
		},
		{
			name:          "Deck msg type",
			inputFilePath: "testdata/deck_msg.json", // Mod ref - https://github.com/Spireblight/STR-Extension/blob/c069792cdda4c7a01266cc41f2ea8b1583162d4b/twitch_demo/deck_msg.txt
			expectedOutput: PubSubMessage{
				MessageType: MessageTypeDeck,
				Streamer: &Streamer{
					Login:  "arrakisdev",
					Secret: "yxk1d4kv3zz5bb4u8byp",
				},
				Metadata: map[string]interface{}{"version": "1.1.0"},
				Delay:    7032,
				MessageContent: MessageContentDeck{
					Character: "Defect",
					Deck:      "your|damage|turn|twice|.;;|next|first|#yLightning|played|#yChannel|Orb|#yEvoke|ability|trigger|the|#yBlock|passive|The &6 card you play NL each &2 is &8|&e start of &0 NL &2, &d &e NL|;0;-;-1;-1;+;0;-;|At &i &g &c of &0 NL &5|;0;-;-1;-1;+;0;|;-;-1;-1;_;1;-;|;&b &0 &5 &a NL |;1;0;1;0;| &e end of |.;-;1;1;|,1,1,1,| cards are |;1;1;|;2;| this | NL | &a | &0 | empty ||0,0,0,0&r1,2,3,4,5,6,7;;;Strike&jDeal #g9 &1&q0;0&vDeal 6 &1&4Defend&l0;Gain #g8 &f.;0&t1;0&vGain 5 &f&4Zap&l1,2;&9 1 &7.;1,2&o2;&9 1 &7&4Dualcast&l3&n&3.;3&o2&n&3&4Echo Form&j&h&x&3.;4;3;3&v4&v#yE&ereal.&x&h&x&3&4Streamline+;0&m_;-&v2;0&v2;Deal 20 &1.&xReduce&wcard's cost&xby 1&wcombat&4Amplify+;0&m_;-&t1;4&vThis &2,&z&5 2&xPower&s&8&x&3&4Loop&j&k&y#g2 times&q2;3&v&k &a&4;Block;Until &5 &2, prevents &1&4Channel;Channeling an&yputs it into&z&6&Aslot. If you have no&Aslots,&z&6&yis automatically &bd to make room&4Lightning;&a: Deals &1 to random enemies&4Evoke;Consume&zrightmost&yand use its Evoke effect&4E&ereal;If&wcard is in&zhand at&p&2, it is exhausted. Exhausted&sremoved from&zdeck until&pcombat.",
				},
			},
		},
		{
			name:          "Okay msg JSON",
			inputFilePath: "testdata/okay_msg.json", // Mod ref - https://github.com/Spireblight/STR-Extension/blob/c069792cdda4c7a01266cc41f2ea8b1583162d4b/twitch_demo/okay_msg.txt
			expectedOutput: PubSubMessage{
				MessageType: MessageTypeOK,
				Streamer: &Streamer{
					Login:  "arrakisdev",
					Secret: "yxk1d4kv3zz5bb4u8byp",
				},
				Metadata:       map[string]interface{}{"version": "1.1.0"},
				Delay:          7045,
				MessageContent: MessageContentUnknown(`""`),
			},
		},
		{
			// should still parse even though we have not implemented the type yet
			name:          "Tips msg JSON",
			inputFilePath: "testdata/tips_msg.json", // Mod ref - https://github.com/Spireblight/STR-Extension/blob/c069792cdda4c7a01266cc41f2ea8b1583162d4b/twitch_demo/tips_msg.txt
			expectedOutput: PubSubMessage{
				MessageType: MessageTypeTips,
				Streamer: &Streamer{
					Login:  "arrakisdev",
					Secret: "yxk1d4kv3zz5bb4u8byp",
				},
				Metadata:       map[string]interface{}{"version": "1.1.0"},
				Delay:          7023,
				MessageContent: MessageContentUnknown(`{"c":"Defect","r":[0,[[0,1,2],[3],[4,5],[6],[7]]],"o":[667,[[8,9],[10,11],[12]]],"p":[18.75,46.39,12.5,29.26,[]],"m":[[68.23,52.78,13.54,22.41,[13]]],"u":[[31.67,46.19,5,8.89,[14]],[22.5,35.19,5,8.89,[15]],[13.33,46.19,5,8.89,[16]]],"w":"damage|Dexterity|.;;|#yPassive|combat|#yEvoke|#yBlock|enemy|first|Potion|random|Lightning|turn|the|Focus|enemies|;&3: At &d end of &c, | &0 to a &a |. NL &5: Deal | &9;Gain #b2 | each &4 | your &8 | &0 to ||Cracked Core;At &d start of each &4, #yChannel #b1 #y&b&2Channel;Channeling an Orb puts it into&lempty slot. If you have no empty slots,&lOrb is automatically &5d to make room&2&b;Orb: Deals&m&a &f&2Neow's Lament;Enemies in&l#b3 &4s will have #b1 HP&2Anchor;Start&kwith #b10 &6&2Block;Until next &c, prevents &0&2Akabeko;Your &8 Attack&kdeals #b8 additional &0&2Omamori;Negate &d next #b2 #rCurses you obtain&2&1&j#y&1&2&1;&1 improves Block gained from cards&2&e&j#y&e&2&e;&e increases &d effectiveness of #yChanneled Orbs&2&9 Slot;Use potions during &4 to gain #gbonuses or #rhinder &f&2Aggressive;This &7 intends to NL #yAttack for #b12 &0.;intents/tip/3;;&b&gdeal #b3&h&7&i#b8&h&7&2Dark&gincrease this Orb's &0 by #b6&i#b6&m&d &7 with lowest HP&2Frost&ggain #b2 &6. NL &5: Gain #b5 &6."}`),
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			var err error

			input := []byte(tc.inputStr)
			if tc.inputFilePath != "" {
				input, err = os.ReadFile(tc.inputFilePath)
				assert.NilError(t, err)
			}

			var result PubSubMessage
			err = json.Unmarshal(input, &result)
			if tc.expectedErr {
				assert.Equal(t, true, err != nil)
				return
			}

			assert.NilError(t, err)

			assert.DeepEqual(t, result, tc.expectedOutput)

			// assert that we can marshal it back to JSON and get the same result
			marshaled, err := json.Marshal(result)
			assert.NilError(t, err)

			// remove html escape bs, note that this escape also gets hit by the broadcaster anyways
			resultStr := strings.ReplaceAll(string(marshaled), `\u0026`, `&`)
			assert.Equal(t, resultStr, string(input))
		})
	}
}
