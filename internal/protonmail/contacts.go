// Package protonmail — contact CRUD operations.
package protonmail

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
	govcard "github.com/emersion/go-vcard"
)

// ContactsClient is the interface consumed by the syncer for Proton contact
// operations. Defined here in the consumer package (go-interfaces: consumer
// owns the interface).
//
// Compile-time check that *Client satisfies ContactsClient.
var _ ContactsClient = (*Client)(nil)

// ContactsClient abstracts Proton contact operations for testability.
type ContactsClient interface {
	// ListContacts returns all contact metadata (no card data).
	ListContacts(ctx context.Context) ([]proton.Contact, error)
	// GetContactVCard returns the decrypted vCard string for a contact.
	GetContactVCard(ctx context.Context, id string) (string, error)
	// CreateContact creates a new contact from a vCard string and returns the
	// new contact ID.
	CreateContact(ctx context.Context, vcard string) (string, error)
	// UpdateContact replaces an existing contact's vCard content.
	UpdateContact(ctx context.Context, id, vcard string) error
	// DeleteContact permanently deletes a contact.
	DeleteContact(ctx context.Context, id string) error
}

// ListContacts returns all Proton contacts (metadata only).
func (c *Client) ListContacts(ctx context.Context) ([]proton.Contact, error) {
	raw, err := c.Raw()
	if err != nil {
		return nil, err
	}

	contacts, err := raw.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proton contacts: %w", err)
	}

	// go-defensive: boundary copy — never expose internal slice to caller.
	out := make([]proton.Contact, len(contacts))
	copy(out, contacts)
	return out, nil
}

// GetContactVCard returns the decrypted/merged vCard string for contactID.
// Cards.Merge handles the bitmask-based encrypted+signed card types.
func (c *Client) GetContactVCard(ctx context.Context, id string) (string, error) {
	raw, err := c.Raw()
	if err != nil {
		return "", err
	}
	kr, err := c.Keyring()
	if err != nil {
		return "", err
	}

	contact, err := raw.GetContact(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get proton contact %q: %w", id, err)
	}

	// Cards.Merge decodes all card types (plain, signed, encrypted, encrypted+signed)
	// using the bitmask flags CardTypeEncrypted and CardTypeSigned.
	vcardData, err := contact.Cards.Merge(kr)
	if err != nil {
		return "", fmt.Errorf("decode contact cards %q: %w", id, err)
	}

	// Encode the merged vcard.Card back to string.
	var buf bytes.Buffer
	if err := govcard.NewEncoder(&buf).Encode(vcardData); err != nil {
		return "", fmt.Errorf("encode contact vcard %q: %w", id, err)
	}
	return buf.String(), nil
}

// CreateContact creates a new Proton contact from a vCard string.
// Returns the new contact ID.
func (c *Client) CreateContact(ctx context.Context, vcard string) (string, error) {
	raw, err := c.Raw()
	if err != nil {
		return "", err
	}

	cards, err := c.buildCards(vcard)
	if err != nil {
		return "", err
	}

	req := proton.CreateContactsReq{
		Contacts: []proton.ContactCards{{Cards: cards}},
	}

	resps, err := raw.CreateContacts(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create proton contact: %w", err)
	}
	if len(resps) == 0 {
		return "", fmt.Errorf("create proton contact: empty response")
	}
	// Each item carries its own API result; a batch can succeed at the HTTP level
	// while an individual contact is rejected.
	if code := resps[0].Response.Code; code != proton.SuccessCode {
		return "", fmt.Errorf("create proton contact rejected (code %d): %s",
			code, resps[0].Response.Message)
	}
	return resps[0].Response.Contact.ID, nil
}

// UpdateContact replaces an existing Proton contact's vCard content.
func (c *Client) UpdateContact(ctx context.Context, id, vcard string) error {
	raw, err := c.Raw()
	if err != nil {
		return err
	}

	cards, err := c.buildCards(vcard)
	if err != nil {
		return err
	}

	if _, err := raw.UpdateContact(ctx, id, proton.UpdateContactReq{Cards: cards}); err != nil {
		return fmt.Errorf("update proton contact %q: %w", id, err)
	}
	return nil
}

// DeleteContact permanently deletes a Proton contact.
func (c *Client) DeleteContact(ctx context.Context, id string) error {
	raw, err := c.Raw()
	if err != nil {
		return err
	}

	// DeleteContactsReq.IDs is the correct field name (not ContactIDs).
	req := proton.DeleteContactsReq{IDs: []string{id}}
	if err := raw.DeleteContacts(ctx, req); err != nil {
		return fmt.Errorf("delete proton contact %q: %w", id, err)
	}
	return nil
}

// signedFields are the ONLY vCard properties Proton permits in the cleartext
// signed card (FN, UID, EMAIL, plus VERSION which is handled separately). Every
// other property — CATEGORIES, N, TEL, ADR, NOTE, ORG, … — MUST go in the
// encrypted card, or the API rejects the contact with code 2001 ("field not
// allowed as readable text or signed vCard").
var signedFields = map[string]bool{
	govcard.FieldFormattedName: true,
	govcard.FieldUID:           true,
	govcard.FieldEmail:         true,
}

// buildCards converts a full vCard string into the two-card structure Proton
// expects: a cleartext signed card (VERSION + identity/email fields) and an
// encrypted+signed card (everything else). A single unsigned/clear card is
// rejected by the API (code 2001/2002, "invalid input").
func (c *Client) buildCards(vcardStr string) (proton.Cards, error) {
	kr, err := c.Keyring()
	if err != nil {
		return nil, err
	}
	return buildContactCards(kr, vcardStr)
}

func buildContactCards(kr *crypto.KeyRing, vcardStr string) (proton.Cards, error) {
	parsed, err := govcard.NewDecoder(strings.NewReader(vcardStr)).Decode()
	if err != nil {
		return nil, fmt.Errorf("parse vcard: %w", err)
	}

	// Proton keeps identity/contactable fields in the cleartext signed card and
	// encrypts the rest. Route each property accordingly; both cards carry their
	// own VERSION.
	signed := govcard.Card{}
	signed.SetValue(govcard.FieldVersion, "4.0")
	encrypted := govcard.Card{}
	encrypted.SetValue(govcard.FieldVersion, "4.0")

	for name, fields := range parsed {
		if name == govcard.FieldVersion {
			continue
		}
		if signedFields[name] {
			signed[name] = fields
		} else {
			encrypted[name] = fields
		}
	}

	// FN is mandatory in the signed card.
	if signed.Value(govcard.FieldFormattedName) == "" {
		signed.SetValue(govcard.FieldFormattedName, deriveFN(parsed))
	}

	// Proton requires every EMAIL to sit in its own unique vCard group (it
	// attaches per-email settings to the group). The group must not be shared
	// with any other property or another email, or the API rejects the contact
	// ("Contact email must have a unique group").
	//
	// Apple/Synology also label emails with a separate `itemN.X-ABLabel`
	// property; since that label lives in its own (encrypted) property, Proton
	// renders the type from the group ordinal + label ("item1" -> "1home"). So
	// fold the label into the EMAIL's own TYPE parameter, drop the X-ABLabel, and
	// assign each email a fresh group that collides with nothing.
	labels := abLabelsByGroup(parsed)
	nonEmailGroups := groupsUsedExcept(parsed, govcard.FieldEmail)
	consumed := map[string]bool{}
	assigned := map[string]bool{}
	counter := 1
	for _, f := range signed[govcard.FieldEmail] {
		orig := f.Group
		if lbl := labels[orig]; lbl != "" {
			setEmailType(f, lbl)
			consumed[orig] = true
		} else {
			dropGenericType(f)
		}
		// Keep the original group only if it is unique; otherwise assign the next
		// free itemN not used by any other property or email.
		g := orig
		if g == "" || nonEmailGroups[g] || assigned[g] {
			for {
				cand := fmt.Sprintf("item%d", counter)
				counter++
				if !nonEmailGroups[cand] && !assigned[cand] {
					g = cand
					break
				}
			}
		}
		f.Group = g
		assigned[g] = true
	}
	// Remove the X-ABLabels we folded into TYPE (labels for other groups — e.g.
	// phones — stay in the encrypted card where they render fine).
	removeABLabels(encrypted, consumed)

	signedData, err := encodeCard(signed)
	if err != nil {
		return nil, err
	}
	encryptedData, err := encodeCard(encrypted)
	if err != nil {
		return nil, err
	}

	signedCard, err := newSignedCard(kr, signedData, false)
	if err != nil {
		return nil, fmt.Errorf("build signed card: %w", err)
	}
	encryptedCard, err := newSignedCard(kr, encryptedData, true)
	if err != nil {
		return nil, fmt.Errorf("build encrypted card: %w", err)
	}

	return proton.Cards{encryptedCard, signedCard}, nil
}

// newSignedCard builds a Proton contact card: always detached-signed, and
// encrypted as well when encrypt is true.
func newSignedCard(kr *crypto.KeyRing, data string, encrypt bool) (*proton.Card, error) {
	msg := crypto.NewPlainMessageFromString(data)

	sig, err := kr.SignDetached(msg)
	if err != nil {
		return nil, fmt.Errorf("sign card: %w", err)
	}
	armoredSig, err := sig.GetArmored()
	if err != nil {
		return nil, fmt.Errorf("armor signature: %w", err)
	}

	card := &proton.Card{Type: proton.CardTypeSigned, Signature: armoredSig, Data: data}
	if encrypt {
		enc, encErr := kr.Encrypt(msg, nil)
		if encErr != nil {
			return nil, fmt.Errorf("encrypt card: %w", encErr)
		}
		armored, armErr := enc.GetArmored()
		if armErr != nil {
			return nil, fmt.Errorf("armor encrypted card: %w", armErr)
		}
		card.Type = proton.CardTypeSigned | proton.CardTypeEncrypted
		card.Data = armored
	}
	return card, nil
}

func encodeCard(card govcard.Card) (string, error) {
	var buf bytes.Buffer
	if err := govcard.NewEncoder(&buf).Encode(card); err != nil {
		return "", fmt.Errorf("encode card vcard: %w", err)
	}
	return buf.String(), nil
}

// groupsUsedExcept returns the set of vCard groups used by properties other than
// exceptName — the groups an email must avoid to stay unique.
func groupsUsedExcept(card govcard.Card, exceptName string) map[string]bool {
	out := map[string]bool{}
	for name, fields := range card {
		if name == exceptName {
			continue
		}
		for _, f := range fields {
			if f.Group != "" {
				out[f.Group] = true
			}
		}
	}
	return out
}

// abLabelKey returns the actual map key for the X-ABLabel property (vCard
// property names are case-insensitive), if present.
func abLabelKey(card govcard.Card) (string, bool) {
	for name := range card {
		if strings.EqualFold(name, "X-ABLabel") {
			return name, true
		}
	}
	return "", false
}

// abLabelsByGroup maps each vCard group to its decoded X-ABLabel value.
func abLabelsByGroup(card govcard.Card) map[string]string {
	out := map[string]string{}
	key, ok := abLabelKey(card)
	if !ok {
		return out
	}
	for _, f := range card[key] {
		if f.Group != "" && f.Value != "" {
			out[f.Group] = decodeABLabel(f.Value)
		}
	}
	return out
}

// decodeABLabel unwraps Apple's `_$!<Home>!$_` label encoding to `Home`.
func decodeABLabel(s string) string {
	if strings.HasPrefix(s, "_$!<") && strings.HasSuffix(s, ">!$_") {
		return s[len("_$!<") : len(s)-len(">!$_")]
	}
	return s
}

// setEmailType sets the EMAIL's TYPE parameter to label (replacing any existing
// TYPE, including the generic "INTERNET").
func setEmailType(f *govcard.Field, label string) {
	if f.Params == nil {
		f.Params = govcard.Params{}
	}
	f.Params[govcard.ParamType] = []string{label}
}

// dropGenericType removes the noise-only "INTERNET" TYPE value so Proton does
// not show it as the email's label.
func dropGenericType(f *govcard.Field) {
	if f.Params == nil {
		return
	}
	kept := f.Params[govcard.ParamType][:0:0]
	for _, t := range f.Params[govcard.ParamType] {
		if !strings.EqualFold(t, "internet") {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(f.Params, govcard.ParamType)
	} else {
		f.Params[govcard.ParamType] = kept
	}
}

// removeABLabels drops X-ABLabel entries whose group is in groups (their label
// has been folded into the email TYPE).
func removeABLabels(card govcard.Card, groups map[string]bool) {
	if len(groups) == 0 {
		return
	}
	key, ok := abLabelKey(card)
	if !ok {
		return
	}
	var kept []*govcard.Field
	for _, f := range card[key] {
		if !groups[f.Group] {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		delete(card, key)
	} else {
		card[key] = kept
	}
}

// deriveFN produces a formatted name when the source vCard lacks FN.
func deriveFN(card govcard.Card) string {
	if n := card.Value(govcard.FieldName); n != "" {
		return strings.TrimSpace(strings.ReplaceAll(n, ";", " "))
	}
	return "Unknown"
}
