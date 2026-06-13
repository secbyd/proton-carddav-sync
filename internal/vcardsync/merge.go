// Package vcardsync provides vCard field-level merge helpers.
//
// Proton models far fewer vCard properties than a typical CardDAV server, so a
// change coming back from Proton must not blow away CardDAV-only properties.
// Two strategies are provided:
//
//   - Overlay: a stateless one-way merge (apply src's properties on top of dst).
//   - ThreeWay: a stateful per-property three-way merge that, given each side's
//     last-synced base, propagates additions, edits, and deletions from either
//     side and resolves genuine conflicts by policy.
package vcardsync

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

// preservedFromDst lists properties always kept from dst (the CardDAV side),
// even when src also defines them: identity/structural fields the other side
// must not rewrite.
var preservedFromDst = map[string]bool{
	vcard.FieldVersion: true,
	vcard.FieldUID:     true,
}

// Overlay returns a new card: a copy of dst in which every property that src
// defines replaces dst's property of the same name, while properties unique to
// dst are preserved. VERSION and UID are always taken from dst.
func Overlay(dst, src vcard.Card) vcard.Card {
	merged := vcard.Card{}
	for name, fields := range dst {
		merged[name] = fields
	}
	for name, fields := range src {
		if preservedFromDst[name] {
			continue
		}
		merged[name] = fields
	}
	return merged
}

// OverlayString parses the dst and src vCard strings, overlays src onto dst (see
// Overlay), and returns the merged vCard encoded as a string.
func OverlayString(dst, src string) (string, error) {
	dstCard, err := decode(dst)
	if err != nil {
		return "", fmt.Errorf("parse base vcard: %w", err)
	}
	srcCard, err := decode(src)
	if err != nil {
		return "", fmt.Errorf("parse incoming vcard: %w", err)
	}
	return encode(Overlay(dstCard, srcCard))
}

// Side identifies which side wins a genuine conflict.
type Side int

const (
	// SideProton resolves conflicts in favour of the Proton value.
	SideProton Side = iota
	// SideCardDAV resolves conflicts in favour of the CardDAV value.
	SideCardDAV
)

// ThreeWay performs a per-property three-way merge.
//
// For each property it compares each side's current value against that side's
// own last-synced base (protonBase / carddavBase) to decide whether that side
// changed the property (an edit, an addition, or a deletion). The outcome:
//
//   - changed on one side only        -> take that side's value (incl. deletion)
//   - changed on both, identically     -> take it
//   - changed on both, differently     -> conflict, resolved by conflictWinner
//   - unchanged on both                -> keep the current value (CardDAV first,
//     as the superset), so CardDAV-only properties survive
//
// Using a per-side base is what makes Proton's lossy round-trip safe: a property
// Proton never models is absent from both protonBase and proton, so it never
// looks "changed" on the Proton side and the CardDAV value is preserved.
//
// VERSION and UID are taken from CardDAV (falling back to Proton). The returned
// slice lists property names that were genuine conflicts.
func ThreeWay(protonBase, carddavBase, proton, carddav vcard.Card, conflictWinner Side) (vcard.Card, []string) {
	merged := vcard.Card{}
	var conflicts []string

	for name := range unionKeys(protonBase, carddavBase, proton, carddav) {
		if name == vcard.FieldVersion || name == vcard.FieldUID || name == vcard.FieldRevision {
			continue // handled explicitly below (not a mergeable/conflicting field)
		}

		pb, cb := protonBase[name], carddavBase[name]
		pr, cd := proton[name], carddav[name]
		changedProton := !fieldsEqual(pb, pr)
		changedCardDAV := !fieldsEqual(cb, cd)

		var chosen []*vcard.Field
		switch {
		case changedProton && changedCardDAV:
			if fieldsEqual(pr, cd) {
				chosen = pr
			} else {
				conflicts = append(conflicts, name)
				if conflictWinner == SideProton {
					chosen = pr
				} else {
					chosen = cd
				}
			}
		case changedProton:
			chosen = pr // includes deletion when pr is empty
		case changedCardDAV:
			chosen = cd
		default:
			// Unchanged on both sides: keep the current value, preferring
			// CardDAV (the superset) so CardDAV-only fields are retained.
			if len(cd) > 0 {
				chosen = cd
			} else {
				chosen = pr
			}
		}

		if len(chosen) > 0 {
			merged[name] = chosen
		}
	}

	setIdentity(merged, vcard.FieldUID, carddav, proton)
	if !setIdentity(merged, vcard.FieldVersion, carddav, proton) {
		merged.SetValue(vcard.FieldVersion, "4.0")
	}
	// REV is a timestamp, not a mergeable field: keep the newer side's value.
	if RevTime(carddav).After(RevTime(proton)) {
		setIdentity(merged, vcard.FieldRevision, carddav, proton)
	} else {
		setIdentity(merged, vcard.FieldRevision, proton, carddav)
	}

	return merged, conflicts
}

// Policy selects how genuine conflicts are resolved.
type Policy int

const (
	// PolicyPreferNewer keeps whichever side's card has the newer REV.
	PolicyPreferNewer Policy = iota
	// PolicyPreferProton always keeps the Proton value.
	PolicyPreferProton
	// PolicyPreferCardDAV always keeps the CardDAV value.
	PolicyPreferCardDAV
)

// ThreeWayString is the string form of ThreeWay. Empty inputs decode to empty
// cards (no base on first sync, or a contact absent on one side). It returns the
// merged vCard, the list of conflicted property names, and an error.
func ThreeWayString(protonBase, carddavBase, proton, carddav string, policy Policy) (string, []string, error) {
	pb, err := decodeOrEmpty(protonBase)
	if err != nil {
		return "", nil, fmt.Errorf("parse proton base: %w", err)
	}
	cb, err := decodeOrEmpty(carddavBase)
	if err != nil {
		return "", nil, fmt.Errorf("parse carddav base: %w", err)
	}
	pr, err := decodeOrEmpty(proton)
	if err != nil {
		return "", nil, fmt.Errorf("parse proton: %w", err)
	}
	cd, err := decodeOrEmpty(carddav)
	if err != nil {
		return "", nil, fmt.Errorf("parse carddav: %w", err)
	}

	var winner Side
	switch policy {
	case PolicyPreferProton:
		winner = SideProton
	case PolicyPreferCardDAV:
		winner = SideCardDAV
	default:
		winner = NewerSide(pr, cd)
	}

	merged, conflicts := ThreeWay(pb, cb, pr, cd, winner)
	out, err := encode(merged)
	if err != nil {
		return "", nil, err
	}
	return out, conflicts, nil
}

// EqualString reports whether two vCards carry the same properties/values,
// ignoring formatting and ordering.
func EqualString(a, b string) (bool, error) {
	ca, err := decodeOrEmpty(a)
	if err != nil {
		return false, err
	}
	cb, err := decodeOrEmpty(b)
	if err != nil {
		return false, err
	}
	for name := range unionKeys(ca, cb) {
		if !fieldsEqual(ca[name], cb[name]) {
			return false, nil
		}
	}
	return true, nil
}

// ProtonRelevantDiff reports whether merged differs from protonCurrent in any
// property Proton already models (present in protonCurrent or protonBase). It is
// used to decide whether a write to Proton is warranted, without churning on
// CardDAV-only properties Proton would just drop again.
func ProtonRelevantDiff(merged, protonCurrent, protonBase string) (bool, error) {
	m, err := decodeOrEmpty(merged)
	if err != nil {
		return false, err
	}
	pc, err := decodeOrEmpty(protonCurrent)
	if err != nil {
		return false, err
	}
	pb, err := decodeOrEmpty(protonBase)
	if err != nil {
		return false, err
	}
	for name := range unionKeys(pc, pb) {
		if name == vcard.FieldVersion {
			continue
		}
		if !fieldsEqual(m[name], pc[name]) {
			return true, nil
		}
	}
	return false, nil
}

func decodeOrEmpty(raw string) (vcard.Card, error) {
	if strings.TrimSpace(raw) == "" {
		return vcard.Card{}, nil
	}
	return decode(raw)
}

// NewerSide reports which side was modified more recently per the REV property,
// for conflictWinner under a "prefer newer" policy. Ties (or missing REVs)
// favour Proton.
func NewerSide(proton, carddav vcard.Card) Side {
	if RevTime(carddav).After(RevTime(proton)) {
		return SideCardDAV
	}
	return SideProton
}

// RevTime parses a card's REV property, returning the zero time if absent or
// unparseable.
func RevTime(card vcard.Card) time.Time {
	field := card.Get(vcard.FieldRevision)
	if field == nil {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "20060102T150405Z", "20060102T150405", "20060102"} {
		if t, err := time.Parse(layout, strings.TrimSpace(field.Value)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// setIdentity copies an identity property (UID/VERSION) from the first source
// that has it, into merged. Returns true if a value was set.
func setIdentity(merged vcard.Card, name string, primary, fallback vcard.Card) bool {
	if f := primary[name]; len(f) > 0 {
		merged[name] = f
		return true
	}
	if f := fallback[name]; len(f) > 0 {
		merged[name] = f
		return true
	}
	return false
}

func unionKeys(cards ...vcard.Card) map[string]struct{} {
	keys := map[string]struct{}{}
	for _, c := range cards {
		for name := range c {
			keys[name] = struct{}{}
		}
	}
	return keys
}

// fieldsEqual reports whether two property groups carry the same values and
// parameters, order-independently.
func fieldsEqual(a, b []*vcard.Field) bool {
	if len(a) != len(b) {
		return false
	}
	ka, kb := fieldKeys(a), fieldKeys(b)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}

func fieldKeys(fields []*vcard.Field) []string {
	keys := make([]string, 0, len(fields))
	for _, f := range fields {
		var b strings.Builder
		b.WriteString(f.Value)
		paramNames := make([]string, 0, len(f.Params))
		for p := range f.Params {
			paramNames = append(paramNames, p)
		}
		sort.Strings(paramNames)
		for _, p := range paramNames {
			vals := append([]string(nil), f.Params[p]...)
			sort.Strings(vals)
			b.WriteString("\x00")
			b.WriteString(p)
			b.WriteString("=")
			b.WriteString(strings.Join(vals, ","))
		}
		keys = append(keys, b.String())
	}
	sort.Strings(keys)
	return keys
}

func decode(raw string) (vcard.Card, error) {
	card, err := vcard.NewDecoder(strings.NewReader(raw)).Decode()
	if err != nil {
		return nil, fmt.Errorf("decode vcard: %w", err)
	}
	return card, nil
}

func encode(card vcard.Card) (string, error) {
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return "", fmt.Errorf("encode vcard: %w", err)
	}
	return buf.String(), nil
}
