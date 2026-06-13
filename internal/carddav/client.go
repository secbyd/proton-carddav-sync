// Package carddav wraps go-webdav's CardDAV client.
package carddav

import (
	"context"
	"fmt"
	"net/http"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
)

// Client wraps the CardDAV address book client.
type Client struct {
	dav         *carddav.Client
	addressBook string
}

// New creates an authenticated CardDAV client for the given address book URL.
func New(url, username, password string) (*Client, error) {
	httpClient := &http.Client{
		Transport: &webdav.HTTPClientTransport{
			InsecureSkipVerify: false,
		},
	}
	_ = httpClient // used below via BasicAuth

	davClient, err := carddav.NewClient(
		webdav.HTTPClientWithBasicAuth(http.DefaultClient, username, password),
		url,
	)
	if err != nil {
		return nil, fmt.Errorf("carddav new client: %w", err)
	}
	return &Client{dav: davClient, addressBook: url}, nil
}

// Contact holds a vCard fetched from CardDAV.
type Contact struct {
	Href string
	Etag string
	Card *vcard.Card
}

// ListContacts returns all contacts in the address book.
func (c *Client) ListContacts(ctx context.Context) ([]Contact, error) {
	objs, err := c.dav.QueryAddressBook(ctx, c.addressBook, &carddav.AddressBookQuery{
		PropFilters: []carddav.PropFilter{{Name: "FN"}},
	})
	if err != nil {
		return nil, fmt.Errorf("carddav query: %w", err)
	}
	contacts := make([]Contact, 0, len(objs))
	for _, obj := range objs {
		contacts = append(contacts, Contact{
			Href: obj.Path,
			Etag: obj.ETag,
			Card: &obj.Card,
		})
	}
	return contacts, nil
}

// PutContact creates or updates a contact at the given href.
func (c *Client) PutContact(ctx context.Context, href string, card *vcard.Card, etag string) (string, error) {
	obj := &carddav.AddressObject{
		Path: href,
		ETag: etag,
		Card: *card,
	}
	newEtag, err := c.dav.PutAddressObject(ctx, c.addressBook, obj)
	if err != nil {
		return "", fmt.Errorf("carddav put: %w", err)
	}
	return newEtag, nil
}

// DeleteContact removes the contact at href.
func (c *Client) DeleteContact(ctx context.Context, href string) error {
	if err := c.dav.RemoveAddressObject(ctx, href); err != nil {
		return fmt.Errorf("carddav delete %s: %w", href, err)
	}
	return nil
}
