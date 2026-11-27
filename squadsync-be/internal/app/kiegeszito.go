package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// The Kiegeszito framework: the mechanics every Kiegeszito shares. Each
// Kiegeszito lives in its own file and plugs in by calling
// registerKiegeszito from an init function, so adding one touches no file
// of another (see the kiegeszito struct for the hooks).
//
//   - Switching: the Szervezo switches each Kiegeszito on or off per
//     Esemeny (SetKiegeszito), only while it is in Egyeztetes or
//     Veglegesitve (kiegeszitoSwitchable). Switching off hides the data and
//     never deletes it; switching back on shows it unchanged. A Kiegeszito
//     can veto being switched off (vetoSwitchOff).
//   - Setup (adding and editing things) is allowed in Egyeztetes and
//     Veglegesitve (requireSetupAllowed). Claiming is allowed only in
//     Veglegesitve, and only for Resztvevok (requireClaimAllowed).
//   - Claims follow participation, inside the transaction of the change:
//     a drop-out releases all of that Felhasznalo's claims (SetVisszajelzes),
//     reopening Egyeztetes freezes them (claiming needs Veglegesitve), and
//     re-confirming drops the claims of everyone who isn't a Resztvevo of
//     the newly confirmed Idopont (ConfirmIdopont). Both go through
//     releaseClaims.
//   - Discord: every enabled Kiegeszito adds a fragment to the summary line
//     of the Esemeny's message (summary). Every use case that changes a
//     Kiegeszito ends with kiegeszitoChanged, which re-renders the message
//     through the refresh job (the one writer of the message).

// Errors of the Kiegeszito framework, shared by every Kiegeszito.
var (
	// ErrKiegeszitoOff means the Kiegeszito is switched off for the
	// Esemeny, so its data is hidden.
	ErrKiegeszitoOff = errors.New("the kiegeszito is switched off")
	// ErrKiegeszitoLocked means Kiegeszitok can't be switched on or off in
	// the Esemeny's state (Elmaradt, Lezajlott).
	ErrKiegeszitoLocked = errors.New("kiegeszitok can't be switched in this esemeny state")
	// ErrKiegeszitoInUse means the Kiegeszito vetoed being switched off.
	// A veto wraps it with the reason.
	ErrKiegeszitoInUse = errors.New("the kiegeszito can't be switched off while it is in use")
	// ErrSetupClosed means a Kiegeszito can't be set up in the Esemeny's
	// state: only in Egyeztetes and Veglegesitve.
	ErrSetupClosed = errors.New("kiegeszitok can't be set up in this esemeny state")
	// ErrClaimingClosed means claiming is refused because the Esemeny isn't
	// Veglegesitve (claims are frozen while it is back in Egyeztetes).
	ErrClaimingClosed = errors.New("claiming is open only while the esemeny is veglegesitve")
	// ErrNotResztvevo means only a Resztvevo may claim, and the claim's
	// holder isn't one.
	ErrNotResztvevo = errors.New("only resztvevok can claim")
	// ErrNotPermitted means the Felhasznalo may not do this to this thing,
	// such as editing a Tetel someone else added.
	ErrNotPermitted = errors.New("not permitted")
)

// kiegeszito is how one Kiegeszito plugs into the framework. Only kind is
// required; a nil hook means the Kiegeszito doesn't take part in it.
type kiegeszito struct {
	kind domain.KiegeszitoKind

	// vetoSwitchOff runs in the transaction before the Kiegeszito is
	// switched off. An error keeps it on and is returned to the Szervezo;
	// wrap ErrKiegeszitoInUse with the reason (Koltsegek while it has
	// Kiadasok).
	vetoSwitchOff func(ctx context.Context, t *kiegeszitoTx) error

	// switchedOn runs in the transaction each time the Kiegeszito goes from
	// off to on. Outside calls (Spotify, #23) belong in t.afterCommit.
	switchedOn func(ctx context.Context, t *kiegeszitoTx) error

	// releaseClaims releases, in the transaction, every claim whose holder
	// release reports true, and returns what was freed in short Hungarian
	// fragments for the Szervezo ("2 db sátor"). It runs on drop-outs (only
	// that Felhasznalo) and on re-confirmation (everyone who isn't a
	// Resztvevo), whether the Kiegeszito is on or off, because claims follow
	// participation. DMs it causes (a Sofor's passengers, #17) go in
	// t.afterCommit. Koltsegek has none: Kiadasok are never touched.
	releaseClaims func(ctx context.Context, t *kiegeszitoTx, release func(domain.ClaimHolder) bool) ([]string, error)

	// summary is the Kiegeszito's short fragment of the summary line in the
	// Esemeny's Discord message ("🎒 2 hiányzik"), "" for none. It runs only
	// while the Kiegeszito is on, outside any transaction, and must not
	// queue work.
	summary func(ctx context.Context, t *kiegeszitoTx) (string, error)
}

// kiegeszitoRegistry holds every registered Kiegeszito. It is filled by
// init functions and copied into each App, so tests can change an App's
// copy (export_test.go).
var kiegeszitoRegistry = map[domain.KiegeszitoKind]kiegeszito{}

// registerKiegeszito plugs a Kiegeszito into the framework. Call it from an
// init function in the Kiegeszito's own file.
func registerKiegeszito(k kiegeszito) {
	if !k.kind.Valid() {
		panic(fmt.Sprintf("register kiegeszito: unknown kind %q", k.kind))
	}
	if _, dup := kiegeszitoRegistry[k.kind]; dup {
		panic(fmt.Sprintf("register kiegeszito: %q registered twice", k.kind))
	}
	kiegeszitoRegistry[k.kind] = k
}

func copyKiegeszitoRegistry() map[domain.KiegeszitoKind]kiegeszito {
	out := make(map[domain.KiegeszitoKind]kiegeszito, len(kiegeszitoRegistry))
	for kind, k := range kiegeszitoRegistry {
		out[kind] = k
	}
	return out
}

// registered returns the App's Kiegeszitok in display order.
func (a *App) registered() []kiegeszito {
	var out []kiegeszito
	for _, kind := range domain.KiegeszitoKinds {
		if k, ok := a.kiegeszitok[kind]; ok {
			out = append(out, k)
		}
	}
	return out
}

// kiegeszitoTx is what a hook gets: the App, the queries (of the use case's
// transaction, except for summary), the Esemeny as the change leaves it, and
// a way to run work once the transaction commits.
type kiegeszitoTx struct {
	a *App
	q store.Queries
	e domain.Esemeny

	after      []func(context.Context)
	resztvevok []domain.Resztvevo
}

// afterCommit queues work, such as a DM, to run once the use case commits
// (see App.afterCommit).
func (t *kiegeszitoTx) afterCommit(job func(context.Context)) {
	t.after = append(t.after, job)
}

// Resztvevok returns the Esemeny's Resztvevok (see resztvevokOf).
func (t *kiegeszitoTx) Resztvevok(ctx context.Context) ([]domain.Resztvevo, error) {
	if t.resztvevok != nil {
		return t.resztvevok, nil
	}
	vs, err := t.q.ListVisszajelzesekOfEsemeny(ctx, t.e.ID)
	if err != nil {
		return nil, err
	}
	t.resztvevok = resztvevokOf(t.e, vs)
	return t.resztvevok, nil
}

// KiegeszitoAllapot is one Kiegeszito of an Esemeny as its page shows it.
type KiegeszitoAllapot struct {
	Kind    domain.KiegeszitoKind
	Enabled bool
	// Switchable tells whether the Szervezo can switch it now.
	Switchable bool
}

// kiegeszitoAllapotok lists every registered Kiegeszito of the Esemeny, in
// display order, with whether it is on.
func (a *App) kiegeszitoAllapotok(ctx context.Context, q store.Queries, e domain.Esemeny) ([]KiegeszitoAllapot, error) {
	enabled, err := q.EnabledKiegeszitok(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	out := []KiegeszitoAllapot{}
	for _, k := range a.registered() {
		out = append(out, KiegeszitoAllapot{Kind: k.kind, Enabled: enabled[k.kind], Switchable: kiegeszitoSwitchable(e, k.kind)})
	}
	return out, nil
}

// kiegeszitoSwitchable tells whether a Kiegeszito can be switched on or off
// in the Esemeny's state: in Egyeztetes and Veglegesitve. Elmaradt and
// Lezajlott freeze them. (#22 may let Koltsegek through in Lezajlott here.)
func kiegeszitoSwitchable(e domain.Esemeny, _ domain.KiegeszitoKind) bool {
	return e.Allapot == domain.Egyeztetes || e.Allapot == domain.Veglegesitve
}

// SetKiegeszito switches one Kiegeszito of an Esemeny on or off. Only the
// Szervezo may: a non-Tag gets ErrNotFound, another Tag ErrNotSzervezo. An
// unknown (or not yet built) Kiegeszito gives ErrInvalidInput, an Esemeny
// that is Elmaradt or Lezajlott ErrKiegeszitoLocked, and a veto an error
// wrapping ErrKiegeszitoInUse. Switching to the state it is already in
// changes nothing.
func (a *App) SetKiegeszito(ctx context.Context, felhasznaloID, esemenyID string, kind domain.KiegeszitoKind, enabled bool) error {
	k, ok := a.kiegeszitok[kind]
	if !ok {
		return fmt.Errorf("%w: unknown kiegeszito %q", ErrInvalidInput, kind)
	}
	var t *kiegeszitoTx
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzervezo(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if !kiegeszitoSwitchable(e, kind) {
			return ErrKiegeszitoLocked
		}
		on, err := q.EnabledKiegeszitok(ctx, e.ID)
		if err != nil {
			return err
		}
		if on[kind] == enabled {
			return nil
		}
		t = &kiegeszitoTx{a: a, q: q, e: e}
		if !enabled && k.vetoSwitchOff != nil {
			if err := k.vetoSwitchOff(ctx, t); err != nil {
				return err
			}
		}
		if err := q.SetKiegeszitoEnabled(ctx, e.ID, kind, enabled, a.clock.Now()); err != nil {
			return err
		}
		if enabled && k.switchedOn != nil {
			return k.switchedOn(ctx, t)
		}
		return nil
	})
	if err != nil || t == nil {
		return err
	}
	a.kiegeszitoChanged(esemenyID, t.after...)
	return nil
}

// kiegeszitoChanged re-renders the Esemeny's Discord messages (for the
// summary line) once the use case commits, after running the hooks' queued
// work. Every use case that changes a Kiegeszito calls it.
func (a *App) kiegeszitoChanged(esemenyID string, after ...func(context.Context)) {
	a.afterCommit(func(ctx context.Context) {
		for _, job := range after {
			job(ctx)
		}
		a.refreshEsemenyMessages(ctx, esemenyID)
	})
}

// requireKiegeszitoOn returns the Esemeny if the Felhasznalo is a Tag of
// its Csoport (ErrNotFound otherwise) and the Kiegeszito is on
// (ErrKiegeszitoOff otherwise). Every use case of a Kiegeszito starts here.
func requireKiegeszitoOn(ctx context.Context, q store.Queries, esemenyID, felhasznaloID string, kind domain.KiegeszitoKind) (domain.Esemeny, error) {
	e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
	if err != nil {
		return domain.Esemeny{}, err
	}
	on, err := q.EnabledKiegeszitok(ctx, e.ID)
	if err != nil {
		return domain.Esemeny{}, err
	}
	if !on[kind] {
		return domain.Esemeny{}, ErrKiegeszitoOff
	}
	return e, nil
}

// setupAllowed tells whether Kiegeszitok can be set up (things added,
// edited, deleted) in the Esemeny's state: Egyeztetes and Veglegesitve.
func setupAllowed(e domain.Esemeny) bool {
	return e.Allapot == domain.Egyeztetes || e.Allapot == domain.Veglegesitve
}

// requireSetupAllowed refuses setup outside Egyeztetes and Veglegesitve
// with ErrSetupClosed. Who may set up is each Kiegeszito's own rule.
func requireSetupAllowed(e domain.Esemeny) error {
	if !setupAllowed(e) {
		return ErrSetupClosed
	}
	return nil
}

// requireClaimAllowed lets actorID make or change a claim for holder:
//
//   - only while the Esemeny is Veglegesitve (ErrClaimingClosed), so claims
//     are frozen while it is back in Egyeztetes;
//   - for themselves (ErrNotPermitted for another Felhasznalo), or (#13)
//     for a name-only Vendeg they added, which #13 checks here; until then
//     a Vendeg is ErrNotFound;
//   - only if the holder is a Resztvevo (ErrNotResztvevo).
func requireClaimAllowed(ctx context.Context, q store.Queries, e domain.Esemeny, actorID string, holder domain.ClaimHolder) error {
	if e.Allapot != domain.Veglegesitve {
		return ErrClaimingClosed
	}
	if holder.VendegID != "" {
		return fmt.Errorf("%w: no such vendeg", ErrNotFound)
	}
	if holder.FelhasznaloID != actorID {
		return ErrNotPermitted
	}
	ok, err := isResztvevo(ctx, q, e, holder)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotResztvevo
	}
	return nil
}

// canClaim is requireClaimAllowed as a yes or no, for views.
func canClaim(ctx context.Context, q store.Queries, e domain.Esemeny, felhasznaloID string) (bool, error) {
	err := requireClaimAllowed(ctx, q, e, felhasznaloID, domain.ClaimHolder{FelhasznaloID: felhasznaloID})
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrClaimingClosed), errors.Is(err, ErrNotResztvevo), errors.Is(err, ErrNotPermitted):
		return false, nil
	default:
		return false, err
	}
}

// isResztvevo tells whether the holder is a Resztvevo of the Esemeny.
func isResztvevo(ctx context.Context, q store.Queries, e domain.Esemeny, holder domain.ClaimHolder) (bool, error) {
	vs, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return false, err
	}
	for _, r := range resztvevokOf(e, vs) {
		if domain.HolderOf(r) == holder {
			return true, nil
		}
	}
	return false, nil
}

// releaseClaims runs every Kiegeszito's releaser in the transaction for the
// holders release picks. It returns what was freed in the Kiegeszitok that
// are on (a hidden one isn't worth mentioning) and the work the releasers
// queued for after the commit.
func (a *App) releaseClaims(ctx context.Context, q store.Queries, e domain.Esemeny, release func(domain.ClaimHolder) bool) ([]string, []func(context.Context), error) {
	enabled, err := q.EnabledKiegeszitok(ctx, e.ID)
	if err != nil {
		return nil, nil, err
	}
	t := &kiegeszitoTx{a: a, q: q, e: e}
	freed := []string{}
	for _, k := range a.registered() {
		if k.releaseClaims == nil {
			continue
		}
		f, err := k.releaseClaims(ctx, t, release)
		if err != nil {
			return nil, nil, fmt.Errorf("release %s claims: %w", k.kind, err)
		}
		if enabled[k.kind] {
			freed = append(freed, f...)
		}
	}
	return freed, t.after, nil
}

// releaseDropOut releases every claim of a Felhasznalo who dropped from
// igen (see SetVisszajelzes).
func (a *App) releaseDropOut(ctx context.Context, q store.Queries, e domain.Esemeny, felhasznaloID string) ([]string, []func(context.Context), error) {
	holder := domain.ClaimHolder{FelhasznaloID: felhasznaloID}
	return a.releaseClaims(ctx, q, e, func(h domain.ClaimHolder) bool { return h == holder })
}

// releaseNonResztvevok drops the claims of everyone who isn't a Resztvevo
// of the Esemeny as confirmed (see ConfirmIdopont). e must already be
// Veglegesitve with its new confirmed Idopont.
func (a *App) releaseNonResztvevok(ctx context.Context, q store.Queries, e domain.Esemeny) ([]func(context.Context), error) {
	vs, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	stays := map[domain.ClaimHolder]bool{}
	for _, r := range resztvevokOf(e, vs) {
		stays[domain.HolderOf(r)] = true
	}
	_, after, err := a.releaseClaims(ctx, q, e, func(h domain.ClaimHolder) bool { return !stays[h] })
	return after, err
}

// kiegeszitoSummary joins the fragments of the Kiegeszitok that are on, in
// display order, for the Esemeny's Discord message. "" for none.
func (a *App) kiegeszitoSummary(ctx context.Context, q store.Queries, e domain.Esemeny) (string, error) {
	enabled, err := q.EnabledKiegeszitok(ctx, e.ID)
	if err != nil {
		return "", err
	}
	t := &kiegeszitoTx{a: a, q: q, e: e}
	var parts []string
	for _, k := range a.registered() {
		if !enabled[k.kind] || k.summary == nil {
			continue
		}
		s, err := k.summary(ctx, t)
		if err != nil {
			return "", fmt.Errorf("%s summary: %w", k.kind, err)
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " · "), nil
}
