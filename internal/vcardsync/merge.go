package vcardsync

import (
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

// Strategy controls how conflicts are resolved.
type Strategy string

const (
	PreferProton   Strategy = "prefer-proton"
	PreferCardDAV  Strategy = "prefer-carddav"
	PreferNewer    Strategy = "prefer-newer"
)

// Merge performs a three-way merge of two vCard strings (protonVCard and
// cardDAVVCard), using base as the last-known common ancestor.
// On conflict the chosen strategy wins.
func Merge(base, protonVCard, cardDAVVCard string, strategy Strategy) (string, error) {
	protonCard, err := decodeVCard(protonVCard)
	if err != nil {
		return "", err
	}

	cdCard, err := decodeVCard(cardDAVVCard)
	if err != nil {
		return "", err
	}

	// Simple field-level merge: start with Proton card, overlay CardDAV
	// fields that are newer or according to strategy.
	var winner vcard.Card
	switch strategy {
	case PreferCardDAV:
		winner = mergeFavoring(protonCard, cdCard)
	case PreferNewer:
		protonRev := getRevision(protonCard)
		cdRev := getRevision(cdCard)
		if cdRev.After(protonRev) {
			winner = mergeFavoring(protonCard, cdCard)
		} else {
			winner = mergeFavoring(cdCard, protonCard)
		}
	default: // prefer-proton
		winner = mergeFavoring(cdCard, protonCard)
	}

	return encodeVCard(winner)
}

// MergeNew returns a copy of the given vCard ensuring it has a UID.
func MergeNew(vcardData string) (string, error) {
	card, err := decodeVCard(vcardData)
	if err != nil {
		return "", err
	}
	// If no UID, add a UUID-style one.
	if card.Get(vcard.FieldUID) == nil {
		card.SetValue(vcard.FieldUID, generateUID())
	}
	return encodeVCard(card)
}

// GetUID returns the UID field value from a vCard string.
func GetUID(vcardData string) (string, error) {
	card, err := decodeVCard(vcardData)
	if err != nil {
		return "", err
	}
	f := card.Get(vcard.FieldUID)
	if f == nil {
		return "", nil
	}
	return f.Value, nil
}

// ---------- helpers ---------------------------------------------------------

func mergeFavoring(base, override vcard.Card) vcard.Card {
	merged := make(vcard.Card)
	for k, fields := range base {
		merged[k] = fields
	}
	// Override with winner fields.
	for k, fields := range override {
		if k == vcard.FieldVersion {
			continue
		}
		merged[k] = fields
	}
	return merged
}

func getRevision(card vcard.Card) time.Time {
	f := card.Get(vcard.FieldRevision)
	if f == nil {
		return time.Time{}
	}
	t, err := time.Parse("20060102T150405Z", f.Value)
	if err != nil {
		t, err = time.Parse(time.RFC3339, f.Value)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

func decodeVCard(data string) (vcard.Card, error) {
	return vcard.NewDecoder(strings.NewReader(data)).Decode()
}

func encodeVCard(card vcard.Card) (string, error) {
	var buf strings.Builder
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func generateUID() string {
	// Deterministic-enough UID using current time nanoseconds.
	return "proton-sync-" + time.Now().Format("20060102-150405.000000000")
}
