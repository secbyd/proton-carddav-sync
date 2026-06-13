package carddav

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
)

// Client wraps go-webdav's CardDAV client.
type Client struct {
	client      *carddav.Client
	collURL     string
}

// New creates an authenticated CardDAV client.
func New(serverURL, username, password string) (*Client, error) {
	httpClient := webdav.HTTPClientWithBasicAuth(http.DefaultClient, username, password)
	c, err := carddav.NewClient(httpClient, serverURL)
	if err != nil {
		return nil, fmt.Errorf("new carddav client: %w", err)
	}
	return &Client{client: c, collURL: serverURL}, nil
}

// ContactEntry is a CardDAV contact with its href, etag, and vCard data.
type ContactEntry struct {
	Href  string
	ETag  string
	VCard string
}

// ListAll fetches all contacts from the CardDAV collection.
func (c *Client) ListAll(ctx context.Context) ([]*ContactEntry, error) {
	query := &carddav.AddressBookQuery{
		PropFilters: []carddav.PropFilter{},
	}

	objects, err := c.client.QueryAddressBook(ctx, c.collURL, query)
	if err != nil {
		return nil, fmt.Errorf("carddav query: %w", err)
	}

	entries := make([]*ContactEntry, 0, len(objects))
	for _, obj := range objects {
		var buf bytes.Buffer
		for _, card := range obj.Data {
			if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
				return nil, fmt.Errorf("encode vcard: %w", err)
			}
		}
		entries = append(entries, &ContactEntry{
			Href:  obj.Path,
			ETag:  obj.ETag,
			VCard: buf.String(),
		})
	}
	return entries, nil
}

// Put creates or updates a contact at the given href (relative to collection).
// If href is empty a new path is derived from the UID.
func (c *Client) Put(ctx context.Context, href, vcardData string) (string, error) {
	card, err := vcard.NewDecoder(strings.NewReader(vcardData)).Decode()
	if err != nil {
		return "", fmt.Errorf("parse vcard: %w", err)
	}

	if href == "" {
		uid := card.Get(vcard.FieldUID)
		if uid == nil || uid.Value == "" {
			return "", fmt.Errorf("vcard missing UID")
		}
		href = path.Join(c.collURL, uid.Value+".vcf")
	}

	addr := &carddav.AddressObject{
		Path: href,
		Data: []vcard.Card{card},
	}

	etag, err := c.client.PutAddressObject(ctx, href, addr)
	if err != nil {
		return "", fmt.Errorf("put carddav object at %q: %w", href, err)
	}
	return etag, nil
}

// Delete removes a contact by its href.
func (c *Client) Delete(ctx context.Context, href string) error {
	if err := c.client.DeleteAddressObject(ctx, href); err != nil {
		return fmt.Errorf("delete carddav object %q: %w", href, err)
	}
	return nil
}
