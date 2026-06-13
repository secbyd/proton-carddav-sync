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

// GetContactVCard returns the decrypted vCard string for contactID.
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

	for _, card := range contact.Cards {
		if card.Type == proton.CardTypeSigned || card.Type == proton.CardTypeEncryptedAndSigned {
			decrypted, err := card.Decrypt(kr)
			if err != nil {
				return "", fmt.Errorf("decrypt contact card %q: %w", id, err)
			}
			return decrypted, nil
		}
	}

	return contact.Cards[0].Data, nil
}

// CreateContact creates a new Proton contact from a vCard string.
// Returns the new contact ID.
func (c *Client) CreateContact(ctx context.Context, vcard string) (string, error) {
	raw, err := c.Raw()
	if err != nil {
		return "", err
	}

	req := proton.CreateContactsReq{
		Contacts: []proton.ContactCards{
			{Cards: []proton.Card{{Type: proton.CardTypeCleartext, Data: vcard}}},
		},
	}

	resps, err := raw.CreateContacts(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create proton contact: %w", err)
	}
	if len(resps) == 0 {
		return "", fmt.Errorf("create proton contact: empty response")
	}
	return resps[0].Contact.ID, nil
}

// UpdateContact replaces an existing Proton contact's vCard content.
func (c *Client) UpdateContact(ctx context.Context, id, vcard string) error {
	raw, err := c.Raw()
	if err != nil {
		return err
	}

	req := proton.UpdateContactReq{
		Cards: []proton.Card{{Type: proton.CardTypeCleartext, Data: vcard}},
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

	req := proton.DeleteContactsReq{ContactIDs: []string{id}}
	if err := raw.DeleteContacts(ctx, req); err != nil {
		return fmt.Errorf("delete proton contact %q: %w", id, err)
	}
	return nil
}
