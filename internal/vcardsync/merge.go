// Package vcardsync provides vCard merge and conflict-resolution helpers.
package vcardsync

import (
	"strings"

	"github.com/emersion/go-vcard"
)

// Strategy controls how conflicts are resolved.
type Strategy int

const (
	// PreferProton uses ProtonMail's version on conflict.
	PreferProton Strategy = iota
	// PreferCardDAV uses the CardDAV version on conflict.
	PreferCardDAV
	// PreferNewer chooses the version with the more recent REV field.
	PreferNewer
)

// Merge combines base, localChange, and remoteChange vCards and returns the
// merged result. Fields present in only one side are included unconditionally;
// differing fields are resolved according to strategy.
func Merge(base, local, remote *vcard.Card, strategy Strategy) *vcard.Card {
	merged := vcard.Card{}

	// Collect all field names
	allFields := map[string]struct{}{}
	for f := range *local {
		allFields[f] = struct{}{}
	}
	for f := range *remote {
		allFields[f] = struct{}{}
	}

	for field := range allFields {
		localVals := (*local)[field]
		remoteVals := (*remote)[field]

		switch {
		case len(localVals) == 0:
			merged[field] = remoteVals
		case len(remoteVals) == 0:
			merged[field] = localVals
		case vcardFieldsEqual(localVals, remoteVals):
			merged[field] = localVals
		default:
			// Conflict — apply strategy
			switch strategy {
			case PreferCardDAV:
				merged[field] = remoteVals
			case PreferNewer:
				if revNewer(local, remote) {
					merged[field] = localVals
				} else {
					merged[field] = remoteVals
				}
			default: // PreferProton
				merged[field] = localVals
			}
		}
	}
	return &merged
}

func vcardFieldsEqual(a, b []*vcard.Field) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Value != b[i].Value {
			return false
		}
	}
	return true
}

// revNewer returns true if local has a newer REV than remote.
func revNewer(local, remote *vcard.Card) bool {
	lRev := strings.TrimSpace(local.Get(vcard.FieldRevision).Value)
	rRev := strings.TrimSpace(remote.Get(vcard.FieldRevision).Value)
	// ISO 8601 timestamps sort lexicographically
	return lRev > rRev
}
