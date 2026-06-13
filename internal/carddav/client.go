// Package carddav wraps go-webdav's CardDAV client.
package carddav

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
)

// Sentinel errors.
var (
	// ErrAddressBookNotFound is returned when no address book is found on the
	// server.
	ErrAddressBookNotFound = errors.New("no address book found on server")
)

// ContactsClient is the interface consumed by the syncer for CardDAV contact
// operations (go-interfaces: consumer owns the interface).
//
// Compile-time check that *Client satisfies ContactsClient.
var _ ContactsClient = (*Client)(nil)

// ContactsClient abstracts CardDAV operations for testability.
type ContactsClient interface {
	// ListContacts returns all contacts in the address book.
	ListContacts(ctx context.Context) ([]carddav.AddressObject, error)
	// PutContact creates or updates a contact by UID.
	PutContact(ctx context.Context, uid, vcardStr string) error
	// DeleteContact removes a contact by path.
	DeleteContact(ctx context.Context, path string) error
}

// Client is a CardDAV contact client.
// All methods are safe to call from a single goroutine.
type Client struct {
	inner       *carddav.Client
	addressBook string
	bookCount   int
}

// AddressBook returns the path of the address book this client operates on.
func (c *Client) AddressBook() string { return c.addressBook }

// AddressBookCount returns how many address books were discovered on the server
// (only the first is used).
func (c *Client) AddressBookCount() int { return c.bookCount }

// New creates and connects a CardDAV client, resolving the first address book
// found on the server.
func New(ctx context.Context, serverURL, username, password string) (*Client, error) {
	httpClient := &http.Client{}
	if username != "" {
		httpClient.Transport = basicAuthTransport{
			base:     http.DefaultTransport,
			username: username,
			password: password,
		}
	}

	client, err := carddav.NewClient(httpClient, serverURL)
	if err != nil {
		return nil, fmt.Errorf("create carddav client for %q: %w", serverURL, err)
	}

	// Discover the principal's address book path.
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return nil, fmt.Errorf("find carddav principal: %w", err)
	}

	homeSet, err := client.FindAddressBookHomeSet(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("find address book home set: %w", err)
	}

	books, err := client.FindAddressBooks(ctx, homeSet)
	if err != nil {
		return nil, fmt.Errorf("find address books: %w", err)
	}

	if len(books) == 0 {
		return nil, ErrAddressBookNotFound
	}

	return &Client{
		inner:       client,
		addressBook: books[0].Path,
		bookCount:   len(books),
	}, nil
}

// ListContacts returns all contacts in the address book.
// The returned slice is always non-nil (go-defensive).
//
// The query filters on "UID is defined" rather than sending an empty filter:
// every vCard has a UID, so this matches all contacts, while an empty filter is
// interpreted as "match nothing" by some servers (e.g. Radicale), which would
// silently return zero contacts.
func (c *Client) ListContacts(ctx context.Context) ([]carddav.AddressObject, error) {
	objects, err := c.inner.QueryAddressBook(ctx, c.addressBook, &carddav.AddressBookQuery{
		DataRequest: carddav.AddressDataRequest{AllProp: true},
		PropFilters: []carddav.PropFilter{{Name: "UID"}},
	})
	if err != nil {
		return nil, fmt.Errorf("query address book: %w", err)
	}

	// go-defensive: boundary copy.
	out := make([]carddav.AddressObject, len(objects))
	copy(out, objects)
	return out, nil
}

// PutContact creates or updates a contact at <addressBook>/<uid>.vcf.
//
// go-webdav v0.5.0: PutAddressObject accepts a vcard.Card value (not
// *AddressObject). We parse the raw vCard string into a vcard.Card and
// hand it to the upstream client.
func (c *Client) PutContact(ctx context.Context, uid, vcardStr string) error {
	path := strings.TrimRight(c.addressBook, "/") + "/" + uid + ".vcf"

	card, err := vcard.NewDecoder(strings.NewReader(vcardStr)).Decode()
	if err != nil {
		return fmt.Errorf("parse vcard for %q: %w", uid, err)
	}

	if _, err := c.inner.PutAddressObject(ctx, path, card); err != nil {
		return fmt.Errorf("put carddav contact %q: %w", uid, err)
	}
	return nil
}

// DeleteContact removes a contact by its server path.
//
// go-webdav v0.5.0: deletion is done via the embedded webdav.Client which
// provides a RemoveAll method. The carddav.Client embeds *webdav.Client, so
// we call webdav.Client.RemoveAll directly.
func (c *Client) DeleteContact(ctx context.Context, path string) error {
	if err := c.inner.RemoveAll(ctx, path); err != nil {
		return fmt.Errorf("delete carddav contact at %q: %w", path, err)
	}
	return nil
}

// basicAuthTransport injects HTTP Basic Auth credentials into every request.
type basicAuthTransport struct {
	base     http.RoundTripper
	username string
	password string
}

func (t basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.SetBasicAuth(t.username, t.password)
	return t.base.RoundTrip(r)
}
