package slaytherelics

import (
	"encoding/json"
)

// MessageType type of message being received from the Java mod (as JSON).
// Mod ref - https://github.com/Spireblight/STR-Spire-Mod/blob/17f3cc9fa79c01444f62201bd7901861c913ff9e/src/main/java/str_exporter/builders/JSONMessageBuilder.java#L18
type MessageType int

const (
	_               MessageType = iota
	MessageTypeTips             // 1 - tooltips for relics, potions, cards, etc. Mod ref - https://github.com/Spireblight/STR-Spire-Mod/blob/17f3cc9fa79c01444f62201bd7901861c913ff9e/src/main/java/str_exporter/builders/TipsJSONBuilder.java#L42
	_
	_
	MessageTypeDeck // 4 - deck card info for the overlay. Mod ref - https://github.com/Spireblight/STR-Spire-Mod/blob/17f3cc9fa79c01444f62201bd7901861c913ff9e/src/main/java/str_exporter/builders/DeckJSONBuilder.java#L33
	MessageTypeOK   // 5 - basically our keep-alive message. Mod ref - https://github.com/Spireblight/STR-Spire-Mod/blob/17f3cc9fa79c01444f62201bd7901861c913ff9e/src/main/java/str_exporter/SlayTheRelicsExporter.java#L114
)

// MessageContentUnknown placeholder for unknown message content type.
type MessageContentUnknown []byte

// MarshalJSON implements json.Marshaler for MessageContentUnknown. Just so we don't have to remarshal something we don't need to parse
func (mc MessageContentUnknown) MarshalJSON() ([]byte, error) {
	return mc, nil
}

// MessageContentDeck message content structure for MessageTypeDeck. Mod ref - https://github.com/Spireblight/STR-Spire-Mod/blob/17f3cc9fa79c01444f62201bd7901861c913ff9e/src/main/java/str_exporter/builders/DeckJSONBuilder.java#L59
type MessageContentDeck struct {
	Character string `json:"c,omitempty"` // Character name string.
	Deck      string `json:"k,omitempty"` // Includes all info about tooltips for each card. TODO: Implement struct that manages parsing this
}

// Streamer auth token info for a given message. Mod ref - https://github.com/Spireblight/STR-Spire-Mod/blob/17f3cc9fa79c01444f62201bd7901861c913ff9e/src/main/java/str_exporter/client/Message.java#L12
type Streamer struct {
	Login  string `json:"login,omitempty"`
	Secret string `json:"secret,omitempty"`
}

// PubSubMessage message format for incoming messages from the Java mod. Tags are used for JSON encoding, not decoding
// Mod refs:
// - https://github.com/Spireblight/STR-Spire-Mod/blob/17f3cc9fa79c01444f62201bd7901861c913ff9e/src/main/java/str_exporter/client/Message.java#L5
// - https://github.com/Spireblight/STR-Spire-Mod/blob/main/src/main/java/str_exporter/builders/JSONMessageBuilder.java
type PubSubMessage struct {
	MessageType    MessageType    `json:"msg_type,omitempty"`
	Streamer       *Streamer      `json:"streamer,omitempty"` // Pointer so that it gets omitted if empty
	Metadata       map[string]any `json:"meta,omitempty"`     // Keep as map as this can include stuff other than version that we would want to forward.
	Delay          int            `json:"delay,omitempty"`
	MessageContent interface{}    `json:"message,omitempty"` // Keep interface so that we can still pass through the message as is without having to implement a type for it.
}

// UnmarshalJSON implements json.Unmarshaler for PubSubMessage. Used to handle the casting separately for later use.
func (psm *PubSubMessage) UnmarshalJSON(data []byte) (err error) {
	type Alias PubSubMessage

	// unmarshal into proxy
	aux := &struct {
		MessageContent json.RawMessage `json:"message,omitempty"` // will take priority on marshalling for this obj over PubSubMessage.MessageContent
		*Alias
	}{
		Alias: (*Alias)(psm),
	}

	// we have to reflect, gross but we are using JSON and not msgp
	err = json.Unmarshal(data, &aux)
	if err != nil {
		return err
	}

	// check if we have a message content
	if aux.MessageContent != nil {
		// TODO: implement rest of message types when necessary
		switch psm.MessageType {
		case MessageTypeDeck:
			mc := MessageContentDeck{} // need to unmarshal into non-interface type

			err := json.Unmarshal(aux.MessageContent, &mc)
			if err != nil {
				return err
			}

			psm.MessageContent = mc

		default:
			// this has the added benefit that we don't have to remarshal it as a map later if we don't actually need to read the data
			psm.MessageContent = MessageContentUnknown(aux.MessageContent)
		}
	}

	return nil
}
