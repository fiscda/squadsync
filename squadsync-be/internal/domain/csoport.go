package domain

import "time"

// Csoport is a persistent group of Felhasznalok.
type Csoport struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// Tag is one Felhasznalo's membership in one Csoport. Admin is the role that
// lets them manage the Csoport itself.
type Tag struct {
	CsoportID     string
	FelhasznaloID string
	Admin         bool
	CreatedAt     time.Time
}

// TagWithName is a Tag together with its Felhasznalo's display name.
type TagWithName struct {
	Tag
	FelhasznaloName string
}

// CsoportSummary is a Csoport as one of its Tags sees it in a list.
type CsoportSummary struct {
	Csoport
	// Admin tells whether that Tag is an Admin.
	Admin    bool
	TagCount int
}

// CsoportMeghivo is a reusable link that makes whoever opens it and logs in a
// Tag of one Csoport. It never expires; it only stops working once revoked.
type CsoportMeghivo struct {
	ID        string
	CsoportID string
	// Token is the unguessable secret in the link.
	Token     string
	CreatedBy string
	CreatedAt time.Time
	// RevokedAt is nil while the link works.
	RevokedAt *time.Time
}

// Revoked tells whether the link has been revoked.
func (m CsoportMeghivo) Revoked() bool { return m.RevokedAt != nil }
