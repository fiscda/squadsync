package app

import (
	"context"

	"squadsync-be/internal/domain"
)

// VetoSwitchOff makes this App's Kiegeszito of the given kind refuse being
// switched off with err (nil allows it again), so tests can check the veto
// hook without a Kiegeszito that has one.
func (a *App) VetoSwitchOff(kind domain.KiegeszitoKind, err error) {
	k := a.kiegeszitok[kind]
	k.vetoSwitchOff = func(context.Context, *kiegeszitoTx) error { return err }
	if err == nil {
		k.vetoSwitchOff = nil
	}
	a.kiegeszitok[kind] = k
}

// SetSwitchable gives this App's Kiegeszito of the given kind its own rule
// for when it can be switched (nil restores the default).
func (a *App) SetSwitchable(kind domain.KiegeszitoKind, switchable func(domain.Esemeny) bool) {
	k := a.kiegeszitok[kind]
	k.switchable = switchable
	a.kiegeszitok[kind] = k
}
