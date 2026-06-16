package protonmail

import (
	"strings"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	govcard "github.com/emersion/go-vcard"
)

func testKeyRing(t *testing.T) *crypto.KeyRing {
	t.Helper()
	key, err := crypto.GenerateKey("Test", "test@example.com", "x25519", 0)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	kr, err := crypto.NewKeyRing(key)
	if err != nil {
		t.Fatalf("NewKeyRing: %v", err)
	}
	return kr
}

func TestBuildContactCards(t *testing.T) {
	kr := testKeyRing(t)

	const src = "BEGIN:VCARD\r\nVERSION:3.0\r\n" +
		"FN:Test 2\r\nN:2;Test;;;\r\n" +
		"EMAIL:a@example.com\r\nEMAIL:b@example.com\r\n" +
		"TEL:+311\r\nUID:uid-123\r\nCATEGORIES:Friends,Work\r\nEND:VCARD\r\n"

	cards, err := buildContactCards(kr, src)
	if err != nil {
		t.Fatalf("buildContactCards: %v", err)
	}

	signed, ok := cards.Get(proton.CardTypeSigned)
	if !ok {
		t.Fatal("no signed card")
	}
	if _, ok := cards.Get(proton.CardTypeSigned | proton.CardTypeEncrypted); !ok {
		t.Fatal("no encrypted+signed card")
	}
	for _, c := range cards {
		if c.Signature == "" {
			t.Errorf("card type %d is unsigned", c.Type)
		}
	}

	// The signed card is cleartext: parse and check identity/email placement.
	sc, err := govcard.NewDecoder(strings.NewReader(signed.Data)).Decode()
	if err != nil {
		t.Fatalf("decode signed card: %v", err)
	}
	if sc.Value(govcard.FieldFormattedName) != "Test 2" {
		t.Errorf("signed FN = %q, want Test 2", sc.Value(govcard.FieldFormattedName))
	}
	if sc.Value(govcard.FieldUID) != "uid-123" {
		t.Errorf("signed UID = %q, want uid-123", sc.Value(govcard.FieldUID))
	}
	// Only FN/UID/EMAIL are allowed in the signed card; CATEGORIES (and N/TEL)
	// must NOT appear there (Proton rejects them — code 2001).
	for _, banned := range []string{govcard.FieldCategories, govcard.FieldName, "TEL"} {
		if sc.Value(banned) != "" {
			t.Errorf("signed card unexpectedly contains %s=%q (must be encrypted)", banned, sc.Value(banned))
		}
	}

	// Every EMAIL must carry a unique, non-empty group (Proton requirement).
	emails := sc[govcard.FieldEmail]
	if len(emails) != 2 {
		t.Fatalf("signed emails = %d, want 2", len(emails))
	}
	groups := map[string]bool{}
	for _, e := range emails {
		if e.Group == "" {
			t.Errorf("email %q has no group", e.Value)
		}
		if groups[e.Group] {
			t.Errorf("duplicate email group %q", e.Group)
		}
		groups[e.Group] = true
	}

	// Round-trip the full contact through Merge and confirm TEL (encrypted) and
	// the identity fields all survive.
	merged, err := cards.Merge(kr)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if merged.Value("TEL") != "+311" {
		t.Errorf("merged TEL = %q, want +311", merged.Value("TEL"))
	}
	if merged.Value(govcard.FieldCategories) == "" {
		t.Error("merged card lost CATEGORIES (must be preserved in the encrypted card)")
	}
	if merged.Value(govcard.FieldFormattedName) != "Test 2" {
		t.Errorf("merged FN = %q, want Test 2", merged.Value(govcard.FieldFormattedName))
	}
}
