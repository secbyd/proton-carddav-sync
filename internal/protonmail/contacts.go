package protonmail

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
)

// Contact is a simplified view of a ProtonMail contact.
type Contact struct {
	ID    string
	Name  string
	VCard string // raw vCard data (decrypted)
}

// ListContacts returns all contacts for the authenticated user.
func (c *Client) ListContacts(ctx context.Context) ([]Contact, error) {
	rawContacts, err := c.client.GetAllContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing proton contacts: %w", err)
	}

	contacts := make([]Contact, 0, len(rawContacts))
	for _, rc := range rawContacts {
		contacts = append(contacts, Contact{
			ID:   rc.ID,
			Name: rc.Name,
		})
	}
	return contacts, nil
}

// GetContactVCard fetches and returns the decrypted vCard for a contact ID.
func (c *Client) GetContactVCard(ctx context.Context, contactID string) (string, error) {
	cards, err := c.client.GetContactCards(ctx, contactID)
	if err != nil {
		return "", fmt.Errorf("getting contact cards %s: %w", contactID, err)
	}
	for _, card := range cards {
		if card.Type == proton.CardTypeCleartext || card.Type == proton.CardTypeSignedEncrypted {
			return card.Data, nil
		}
	}
	return "", fmt.Errorf("no readable card found for contact %s", contactID)
}

// CreateContact creates a new contact from a vCard string.
func (c *Client) CreateContact(ctx context.Context, vcard string) (string, error) {
	req := proton.CreateContactReq{
		Contacts: []proton.ContactCards{
			{
				Cards: []proton.Card{
					{Type: proton.CardTypeCleartext, Data: vcard},
				},
			},
		},
	}
	resps, err := c.client.CreateContacts(ctx, req)
	if err != nil || len(resps) == 0 {
		return "", fmt.Errorf("creating proton contact: %w", err)
	}
	return resps[0].ID, nil
}

// UpdateContact replaces the vCard for an existing contact.
func (c *Client) UpdateContact(ctx context.Context, contactID, vcard string) error {
	req := proton.UpdateContactReq{
		Cards: []proton.Card{
			{Type: proton.CardTypeCleartext, Data: vcard},
		},
	}
	_, err := c.client.UpdateContact(ctx, contactID, req)
	return err
}

// DeleteContact removes a contact by ID.
func (c *Client) DeleteContact(ctx context.Context, contactID string) error {
	_, err := c.client.DeleteContacts(ctx, []string{contactID})
	return err
}
