// Package vcardsync provides vCard field-level merge helpers.
//
// Proton models far fewer vCard properties than a typical CardDAV server, so a
// change coming back from Proton must not blow away CardDAV-only properties.
// Overlay applies the properties Proton carries on top of the existing CardDAV
// card, preserving everything else.
package vcardsync

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/emersion/go-vcard"
)

// preservedFromDst lists properties always kept from dst (the CardDAV side),
// even when src also defines them: identity/structural fields the other side
// must not rewrite.
var preservedFromDst = map[string]bool{
	vcard.FieldVersion: true,
	vcard.FieldUID:     true,
}

// Overlay returns a new card: a copy of dst in which every property that src
// defines replaces dst's property of the same name, while properties unique to
// dst are preserved. This lets changes from src (Proton) update the fields it
// knows about without discarding dst-only (CardDAV-only) fields. VERSION and UID
// are always taken from dst.
func Overlay(dst, src vcard.Card) vcard.Card {
	merged := vcard.Card{}
	for name, fields := range dst {
		merged[name] = fields
	}
	for name, fields := range src {
		if preservedFromDst[name] {
			continue
		}
		merged[name] = fields
	}
	return merged
}

// OverlayString parses the dst and src vCard strings, overlays src onto dst (see
// Overlay), and returns the merged vCard encoded as a string.
func OverlayString(dst, src string) (string, error) {
	dstCard, err := decode(dst)
	if err != nil {
		return "", fmt.Errorf("parse base vcard: %w", err)
	}
	srcCard, err := decode(src)
	if err != nil {
		return "", fmt.Errorf("parse incoming vcard: %w", err)
	}
	return encode(Overlay(dstCard, srcCard))
}

func decode(raw string) (vcard.Card, error) {
	card, err := vcard.NewDecoder(strings.NewReader(raw)).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode vcard: %w", err)
	}
	return card, nil
}

func encode(card vcard.Card) (string, error) {
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return "", fmt.Errorf("encode vcard: %w", err)
	}
	return buf.String(), nil
}
