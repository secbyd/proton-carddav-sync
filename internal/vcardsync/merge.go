// Package vcardsync implements three-way vCard merging strategies.
package vcardsync

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

// Direction controls which side wins when both have been modified.
type Direction int

const (
	// PreferProton resolves conflicts by keeping the Proton version.
	PreferProton Direction = iota + 1 // go-defensive: start iota at 1 so zero == uninitialized
	// PreferCardDAV resolves conflicts by keeping the CardDAV version.
	PreferCardDAV
	// PreferNewer resolves conflicts by keeping the more recently modified
	// version (REV field).
	PreferNewer
)

// Sentinel errors.
var (
	// ErrUnknownDirection is returned when an unknown merge Direction is used.
	ErrUnknownDirection = errors.New("unknown merge direction")
)

// Merge performs a three-way vCard merge given:
//   - base: last known common vCard (may be empty for new contacts)
//   - proton: current Proton version
//   - carddav: current CardDAV version
//   - dir: conflict resolution strategy
//
// Returns the merged vCard string or an error.
func Merge(base, proton, carddav string, dir Direction) (string, error) {
	if dir < PreferProton || dir > PreferNewer {
		return "", ErrUnknownDirection
	}

	// If one side is empty, return the other (new contact scenario).
	if strings.TrimSpace(proton) == "" {
		return carddav, nil
	}
	if strings.TrimSpace(carddav) == "" {
		return proton, nil
	}

	protonCard, err := parseVCard(proton)
	if err != nil {
		return "", fmt.Errorf("parse proton vcard: %w", err)
	}

	carddavCard, err := parseVCard(carddav)
	if err != nil {
		return "", fmt.Errorf("parse carddav vcard: %w", err)
	}

	switch dir {
	case PreferProton:
		return proton, nil
	case PreferCardDAV:
		return carddav, nil
	case PreferNewer:
		protonRev := revTime(protonCard)
		carddavRev := revTime(carddavCard)
		if carddavRev.After(protonRev) {
			return carddav, nil
		}
		return proton, nil
	default:
		return "", ErrUnknownDirection
	}
}

func parseVCard(raw string) (vcard.Card, error) {
	dec := vcard.NewDecoder(strings.NewReader(raw))
	card, err := dec.Decode()
	if err != nil {
		return nil, fmt.Errorf("decode vcard: %w", err)
	}
	return card, nil
}

func revTime(card vcard.Card) time.Time {
	field := card.Get(vcard.FieldRevision)
	if field == nil {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "20060102T150405Z", "20060102"} {
		if t, err := time.Parse(layout, strings.TrimSpace(field.Value)); err == nil {
			return t
		}
	}
	return time.Time{}
}
