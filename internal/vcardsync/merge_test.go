package vcardsync

import (
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
)

// TestOverlayPreservesCardDAVOnlyFields models the real scenario: a rich CardDAV
// contact, of which Proton only round-trips a subset, with the phone number
// changed on the Proton side. The phone must update while CardDAV-only fields
// (NOTE, NICKNAME, an X- extension) survive.
func TestOverlayPreservesCardDAVOnlyFields(t *testing.T) {
	const cardDAV = "BEGIN:VCARD\r\n" +
		"VERSION:4.0\r\n" +
		"UID:contact-123\r\n" +
		"FN:Alice Example\r\n" +
		"TEL;TYPE=cell:+3110000000\r\n" +
		"EMAIL:alice@example.com\r\n" +
		"NICKNAME:Al\r\n" +
		"NOTE:met at conference\r\n" +
		"X-CUSTOM-FIELD:keep me\r\n" +
		"END:VCARD\r\n"

	// What Proton hands back after the phone edit: a reduced card (no NICKNAME,
	// NOTE, or X-CUSTOM-FIELD) with a new TEL.
	const proton = "BEGIN:VCARD\r\n" +
		"VERSION:4.0\r\n" +
		"UID:contact-123\r\n" +
		"FN:Alice Example\r\n" +
		"TEL;TYPE=cell:+3119999999\r\n" +
		"EMAIL:alice@example.com\r\n" +
		"END:VCARD\r\n"

	mergedStr, err := OverlayString(cardDAV, proton)
	if err != nil {
		t.Fatalf("OverlayString: %v", err)
	}

	merged, err := vcard.NewDecoder(strings.NewReader(mergedStr)).Decode()
	if err != nil {
		t.Fatalf("decode merged: %v", err)
	}

	// Proton's change won.
	if got := merged.Value("TEL"); got != "+3119999999" {
		t.Errorf("TEL = %q, want the updated Proton number +3119999999", got)
	}

	// CardDAV-only fields preserved.
	for prop, want := range map[string]string{
		"NICKNAME":       "Al",
		"NOTE":           "met at conference",
		"X-CUSTOM-FIELD": "keep me",
	} {
		if got := merged.Value(prop); got != want {
			t.Errorf("%s = %q, want %q (CardDAV-only field should survive)", prop, got, want)
		}
	}

	// Identity preserved.
	if got := merged.Value(vcard.FieldUID); got != "contact-123" {
		t.Errorf("UID = %q, want contact-123", got)
	}
}

// TestOverlayNoChangeIsStable ensures overlaying an identical subset does not
// alter the preserved fields (supports the syncer's skip-if-unchanged check).
func TestOverlayNoChangeIsStable(t *testing.T) {
	const card = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:u1\r\nFN:Bob\r\nNOTE:keep\r\nEND:VCARD\r\n"
	const subset = "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:u1\r\nFN:Bob\r\nEND:VCARD\r\n"

	merged, err := OverlayString(card, subset)
	if err != nil {
		t.Fatalf("OverlayString: %v", err)
	}
	decoded, err := vcard.NewDecoder(strings.NewReader(merged)).Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := decoded.Value("NOTE"); got != "keep" {
		t.Errorf("NOTE = %q, want keep", got)
	}
}
