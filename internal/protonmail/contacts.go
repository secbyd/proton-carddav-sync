// Package protonmail — contact CRUD operations.
package protonmail

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
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
	import "bytes"
	import govcard "github.com/emersion/go-vcard"
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

	// proton.Cards is []*proton.Card; proton.CardTypeClear is the unencrypted type.
	req := proton.CreateContactsReq{
		Contacts: []proton.ContactCards{
			{Cards: proton.Cards{&proton.Card{Type: proton.CardTypeClear, Data: vcard}}},
		},
	}

	resps, err := raw.CreateContacts(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create proton contact: %w", err)
	}
	if len(resps) == 0 {
		return "", fmt.Errorf("create proton contact: empty response")
	}
	// CreateContactsRes wraps the per-contact result in Response.Contact.
	return resps[0].Response.Contact.ID, nil
}

// UpdateContact replaces an existing Proton contact's vCard content.
func (c *Client) UpdateContact(ctx context.Context, id, vcard string) error {
	raw, err := c.Raw()
	if err != nil {
		return err
	}

	req := proton.UpdateContactReq{
		Cards: proton.Cards{&proton.Card{Type: proton.CardTypeClear, Data: vcard}},
	}

	if _, err := raw.UpdateContact(ctx, id, req); err != nil {
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
