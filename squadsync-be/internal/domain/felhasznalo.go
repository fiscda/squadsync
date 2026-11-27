// Package domain holds the plain data types of the domain, shared by the
// application core, the store and the internal HTTP API. Rules live in the
// application core (package app), not here.
package domain

import "time"

// Felhasznalo is a single human using the app. Its ID is the same as Better
// Auth's user.id (ADR-0002).
type Felhasznalo struct {
	ID        string
	Name      string
	CreatedAt time.Time
}

// DiscordAccount maps one Discord account to its Felhasznalo. Every
// Felhasznalo has exactly one main account and may link more (ADR-0002).
type DiscordAccount struct {
	DiscordID     string
	FelhasznaloID string
	IsMain        bool
	CreatedAt     time.Time
}
