package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/exp/slices"
)

const WILDCARDS = "0123456789abcdefghijklmnopqrstvwxyzABCDEFGHIJKLMNOPQRSTVWXYZ_`[]/^%?@><=-+*:;,.()#$!'{}~"

func (a *API) getDeckHandler(c *gin.Context) {
	name := c.Param("name")
	name = strings.ToLower(name)

	deck, ok := func() (string, bool) {
		a.deckLock.RLock()
		defer a.deckLock.RUnlock()
		deck, ok := a.deckLists[name]
		return deck, ok
	}()

	if !ok {
		c.JSON(404, gin.H{"error": "deck not found"})
		return
	}
	d, err := decompressDeck(deck)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.Data(200, "text/plain", formatDeck(d))
}

// formatDeck convert deck card count map into readable string
func formatDeck(deck map[string]int) []byte {
	keys := make([]string, 0, len(deck))
	for k := range deck {
		keys = append(keys, k)
	}

	// put Ascender's Bane at the end
	slices.SortFunc(keys, func(i, j string) bool {
		if i == "Ascender's Bane" {
			return false
		}
		if j == "Ascender's Bane" {
			return true
		}
		return i < j
	})

	buf := bytes.NewBuffer(make([]byte, 0, 2048)) // typically ends up being around 2KB (for the "large deck" case)
	for _, key := range keys {
		buf.Write([]byte(key))
		buf.Write([]byte(" x"))
		buf.WriteString(strconv.Itoa(deck[key]))
		buf.Write([]byte("\n"))
	}

	return buf.Bytes()
}

func escapeRegexp(s string) string {
	r := regexp.MustCompile(`[-\/\\^$*+?.()|[\]{}]`)
	return r.ReplaceAllString(s, "\\$&")
}

func decompress(s string) (string, error) {
	parts := strings.Split(s, "||")
	if len(parts) < 2 {
		return "", errors.New("invalid deck")
	}

	compressionDict := strings.Split(parts[0], "|")
	text := parts[1]

	for i := len(compressionDict) - 1; i >= 0; i-- {
		word := compressionDict[i]
		wildCard := fmt.Sprintf("&%c", WILDCARDS[i])
		r, err := regexp.Compile(escapeRegexp(wildCard))
		if err != nil {
			return "", err
		}
		text = r.ReplaceAllString(text, word)
	}
	return text, nil
}

func parseCommaDelimitedIntegerArray(s string) ([]int, error) {
	if s == "-" {
		return make([]int, 0), nil
	}
	//nolint:prealloc
	var result []int
	err := json.Unmarshal([]byte(fmt.Sprintf("[%s]", s)), &result)
	return result, err
}

func splitSemicolonDelimited2DArray(s string) [][]string {
	if s == "-" {
		return make([][]string, 0)
	}

	//nolint:prealloc
	var result [][]string
	split := strings.Split(s, ";;")
	for _, element := range split {
		result = append(result, strings.Split(element, ";"))
	}
	return result
}

func parseCards(cards [][]string) []string {
	//nolint:prealloc
	var result []string
	for _, card := range cards {
		result = append(result, parseCard(card))
	}
	return result
}

func parseCard(c []string) string {
	return c[0]
}

func decompressDeck(deck string) (map[string]int, error) {
	deck, err := decompress(deck)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(deck, ";;;")
	d, err := parseCommaDelimitedIntegerArray(parts[0])
	if err != nil {
		return nil, err
	}
	cards := parseCards(splitSemicolonDelimited2DArray(parts[1]))

	deckDict := make(map[string]int)
	for _, idx := range d {
		if idx < 0 || idx >= len(cards) {
			return nil, errors.New("card index out of bounds")
		}

		name := cards[idx]
		deckDict[name]++
	}

	return deckDict, nil
}
