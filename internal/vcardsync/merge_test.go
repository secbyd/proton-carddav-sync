package vcardsync

import (
	"strings"
	"testing"

	"github.com/emersion/go-vcard"
)

// vc builds a minimal vCard string from "PROP:value" lines.
func vc(lines ...string) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:u1\r\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\r\n")
	}
	b.WriteString("END:VCARD\r\n")
	return b.String()
}

func mustThreeWay(t *testing.T, pb, cb, pr, cd string, policy Policy) (vcard.Card, []string) {
	t.Helper()
	mergedStr, conflicts, err := ThreeWayString(pb, cb, pr, cd, policy)
	if err != nil {
		t.Fatalf("ThreeWayString: %v", err)
	}
	card, err := vcard.NewDecoder(strings.NewReader(mergedStr)).Decode()
	if err != nil {
		t.Fatalf("decode merged: %v", err)
	}
	return card, conflicts
}

func TestThreeWay_ProtonEditPreservesCardDAVOnly(t *testing.T) {
	// Base: both sides last agreed on this (Proton's lossy view lacks NOTE).
	protonBase := vc("FN:Alice", "TEL:+311")
	carddavBase := vc("FN:Alice", "TEL:+311", "NOTE:keep")
	// Proton changed the phone; CardDAV unchanged.
	proton := vc("FN:Alice", "TEL:+399")
	carddav := vc("FN:Alice", "TEL:+311", "NOTE:keep")

	card, conflicts := mustThreeWay(t, protonBase, carddavBase, proton, carddav, PolicyPreferNewer)
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %v", conflicts)
	}
	if got := card.Value("TEL"); got != "+399" {
		t.Errorf("TEL = %q, want +399 (Proton edit wins)", got)
	}
	if got := card.Value("NOTE"); got != "keep" {
		t.Errorf("NOTE = %q, want keep (CardDAV-only field preserved)", got)
	}
}

func TestThreeWay_ProtonDeletePropagates(t *testing.T) {
	protonBase := vc("FN:Alice", "TEL:+311")
	carddavBase := vc("FN:Alice", "TEL:+311", "NOTE:keep")
	// Proton removed the phone; CardDAV unchanged.
	proton := vc("FN:Alice")
	carddav := vc("FN:Alice", "TEL:+311", "NOTE:keep")

	card, _ := mustThreeWay(t, protonBase, carddavBase, proton, carddav, PolicyPreferNewer)
	if got := card.Get("TEL"); got != nil {
		t.Errorf("TEL = %q, want deleted (Proton deletion propagates)", got.Value)
	}
	if got := card.Value("NOTE"); got != "keep" {
		t.Errorf("NOTE = %q, want keep", got)
	}
}

func TestThreeWay_LossyRoundTripKeepsCardDAVField(t *testing.T) {
	// Proton never models NOTE: it's absent from protonBase AND proton, while
	// CardDAV has it unchanged. It must be preserved (no false deletion).
	protonBase := vc("FN:Alice")
	carddavBase := vc("FN:Alice", "NOTE:keep")
	proton := vc("FN:Alice")
	carddav := vc("FN:Alice", "NOTE:keep")

	card, _ := mustThreeWay(t, protonBase, carddavBase, proton, carddav, PolicyPreferNewer)
	if got := card.Value("NOTE"); got != "keep" {
		t.Errorf("NOTE = %q, want keep (lossy round-trip must not delete)", got)
	}
}

func TestThreeWay_ConflictPolicy(t *testing.T) {
	base := vc("FN:Alice", "TEL:+311")
	proton := vc("FN:Alice", "TEL:+pp")
	carddav := vc("FN:Alice", "TEL:+cc")

	for _, tc := range []struct {
		name   string
		want   string
		policy Policy
	}{
		{"prefer-proton", "+pp", PolicyPreferProton},
		{"prefer-carddav", "+cc", PolicyPreferCardDAV},
	} {
		t.Run(tc.name, func(t *testing.T) {
			card, conflicts := mustThreeWay(t, base, base, proton, carddav, tc.policy)
			if len(conflicts) != 1 || conflicts[0] != "TEL" {
				t.Fatalf("conflicts = %v, want [TEL]", conflicts)
			}
			if got := card.Value("TEL"); got != tc.want {
				t.Errorf("TEL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestThreeWay_PreferNewerByRev(t *testing.T) {
	base := vc("FN:Alice", "TEL:+311")
	proton := vc("FN:Alice", "TEL:+pp", "REV:20240101T000000Z")
	carddav := vc("FN:Alice", "TEL:+cc", "REV:20250101T000000Z") // newer

	card, conflicts := mustThreeWay(t, base, base, proton, carddav, PolicyPreferNewer)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want one", conflicts)
	}
	if got := card.Value("TEL"); got != "+cc" {
		t.Errorf("TEL = %q, want +cc (newer CardDAV REV wins)", got)
	}
}

func TestProtonRelevantDiff(t *testing.T) {
	protonBase := vc("FN:Alice", "TEL:+311")
	protonCurrent := vc("FN:Alice", "TEL:+311")

	// merged adds only a CardDAV-only NOTE: not Proton-relevant.
	mergedNoteOnly := vc("FN:Alice", "TEL:+311", "NOTE:extra")
	if diff, err := ProtonRelevantDiff(mergedNoteOnly, protonCurrent, protonBase); err != nil || diff {
		t.Errorf("ProtonRelevantDiff(note-only) = %v,%v; want false", diff, err)
	}

	// merged changes the phone: Proton-relevant.
	mergedPhone := vc("FN:Alice", "TEL:+999")
	if diff, err := ProtonRelevantDiff(mergedPhone, protonCurrent, protonBase); err != nil || !diff {
		t.Errorf("ProtonRelevantDiff(phone) = %v,%v; want true", diff, err)
	}
}

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
