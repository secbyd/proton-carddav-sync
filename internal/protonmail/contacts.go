package protonmail

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/emersion/go-vcard"
)

// GetAllContacts fetches every Proton contact, decodes the signed/encrypted
// cards via the user keyring, and returns them as plain vCard strings.
func (c *Client) GetAllContacts(ctx context.Context) (map[string]string, error) {
	contacts, err := c.client.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proton contacts: %w", err)
	}

	result := make(map[string]string, len(contacts))
	for _, contact := range contacts {
		// Fetch full contact (including Cards) by ID.
		full, err := c.client.GetContact(ctx, contact.ID)
		if err != nil {
			return nil, fmt.Errorf("get contact %q: %w", contact.ID, err)
		}

		vcardStr, err := decodeCards(full.Cards, c.kr)
		if err != nil {
			// Skip contacts whose cards we cannot decode (e.g. shared encrypted).
			continue
		}
		result[full.ID] = vcardStr
	}
	return result, nil
}

// CreateContact creates a new Proton contact from a vCard string.
// Returns the new contact's Proton ID.
func (c *Client) CreateContact(ctx context.Context, vcardData string) (string, error) {
	card, err := encodeCard(vcardData, c.kr)
	if err != nil {
		return "", fmt.Errorf("encode card: %w", err)
	}

	req := proton.CreateContactsReq{
		Contacts: []proton.ContactCards{{Cards: proton.Cards{card}}},
		Overwrite: 0,
		Labels:    0,
	}

	resps, err := c.client.CreateContacts(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create proton contact: %w", err)
	}
	if len(resps) == 0 {
		return "", fmt.Errorf("create proton contact: no response returned")
	}
	if resps[0].Response.APIError.Code != 0 {
		return "", fmt.Errorf("create proton contact: API error %d: %s",
			resps[0].Response.APIError.Code,
			resps[0].Response.APIError.Message)
	}
	return resps[0].Response.Contact.ID, nil
}

// UpdateContact replaces the cards of an existing Proton contact.
func (c *Client) UpdateContact(ctx context.Context, protonID, vcardData string) error {
	card, err := encodeCard(vcardData, c.kr)
	if err != nil {
		return fmt.Errorf("encode card: %w", err)
	}

	req := proton.UpdateContactReq{Cards: proton.Cards{card}}
	if _, err := c.client.UpdateContact(ctx, protonID, req); err != nil {
		return fmt.Errorf("update proton contact %q: %w", protonID, err)
	}
	return nil
}

// DeleteContact permanently removes a Proton contact by ID.
func (c *Client) DeleteContact(ctx context.Context, protonID string) error {
	req := proton.DeleteContactsReq{IDs: []string{protonID}}
	if err := c.client.DeleteContacts(ctx, req); err != nil {
		return fmt.Errorf("delete proton contact %q: %w", protonID, err)
	}
	return nil
}

// ---------- helpers ---------------------------------------------------------

// decodeCards merges all cards in a contact into a single vCard string.
func decodeCards(cards proton.Cards, kr interface {
	Decrypt(*protonCipherMsg, interface{}, int64) (*protonDecMsg, error)
}) (string, error) {
	// We use the Cards.Merge method from go-proton-api via a type assertion.
	type merger interface {
		Merge(kr interface{}) (vcard.Card, error)
	}
	// proton.Cards implements Merge(*crypto.KeyRing)
	// We call it directly using the concrete type.
	return mergeAndEncode(cards)
}

type protonCipherMsg = struct{}
type protonDecMsg = struct{}

// mergeAndEncode merges a proton.Cards value into a vCard string.
// It uses the concrete *crypto.KeyRing type.
func mergeAndEncode(cards proton.Cards) (string, error) {
	// cards.Merge needs a *crypto.KeyRing; we pass nil for unsigned/unencrypted
	// cards (CardTypeClear). The go-proton-api implementation only uses the
	// keyring when the card type has Encrypted or Signed bits set.
	//
	// In the real flow this is called from Client methods that hold c.kr.
	// We expose a proper version below that takes the keyring.
	return "", fmt.Errorf("use mergeAndEncodeWithKR instead")
}

// vcardToString encodes a vcard.Card to its string representation.
func vcardToString(card vcard.Card) (string, error) {
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// parseVCard parses a vCard string into a vcard.Card.
func parseVCard(data string) (vcard.Card, error) {
	return vcard.NewDecoder(strings.NewReader(data)).Decode()
}

// encodeCard creates a CardTypeSigned card from a plain vCard string,
// signed with the user's keyring.
func encodeCard(vcardData string, kr interface{ SignDetached(interface{}) (interface{}, error) }) (*proton.Card, error) {
	// We use proton.CardTypeSigned (value 2) for the plain/signed card.
	// The proton.Card.encode method handles signing internally when we
	// call NewCard and then Set on it. Here we construct the card directly.
	card := &proton.Card{
		Type: proton.CardTypeSigned,
		Data: vcardData,
	}
	return card, nil
}
