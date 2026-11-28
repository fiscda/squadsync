package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// The Kiegeszito framework: the mechanics every Kiegeszito shares.
//
//   - Switching: the Szervezo switches each Kiegeszito on or off per
//     Esemeny (SetKiegeszito), by default only in Egyeztetes and
//     Veglegesitve (a Kiegeszito can override this with switchable).
//     Switching off hides the data and never deletes it; switching back on
//     shows it unchanged. A Kiegeszito can veto being switched off
//     (vetoSwitchOff).
//   - Setup (adding and editing things) is allowed in Egyeztetes and
//     Veglegesitve (requireSetupAllowed). Claiming is allowed only in
//     Veglegesitve, and only for Resztvevok (requireClaimAllowed, or
//     requireClaimAllowedBy when the Szervezo may move people).
//   - Claims follow participation, inside the transaction of the change:
//     a drop-out from igen (SetVisszajelzes) and a Tag leaving or being
//     removed from the Csoport (deleteTag) release all of that
//     Felhasznalo's claims; reopening Egyeztetes freezes them (claiming
//     needs Veglegesitve); re-confirming drops the claims of everyone who
//     isn't a Resztvevo of the newly confirmed Idopont (ConfirmIdopont).
//     All of them go through releaseClaims.
//   - Discord: every enabled Kiegeszito adds a fragment to the summary line
//     of the Esemeny's message (summary). Every use case that changes a
//     Kiegeszito ends with kiegeszitoChanged, which re-renders the message
//     through the refresh job (the one writer of the message).
//
// # How to add a Kiegeszito
//
//  1. Migration (your own timestamp). Tables hang off esemeny (id) ON
//     DELETE CASCADE, and every Felhasznalo id column declares REFERENCES
//     felhasznalo (id). A claim table copies tetel_vallalas
//     (202611281601_felszereles.sql): felhasznalo_id REFERENCES
//     felhasznalo (id), vendeg_id TEXT with no REFERENCES yet (#13 adds it),
//     CHECK ((felhasznalo_id IS NULL) <> (vendeg_id IS NULL)) and a
//     partial unique index on each. The kind enum and its CHECK already
//     list all six kinds.
//  2. Store: store/<kind>.go, methods on store.Queries.
//  3. Core: app/<kind>.go registers from its own init, with any of the
//     hooks of the kiegeszito struct below (see their docs):
//
//     func init() {
//     	registerKiegeszito(kiegeszito{
//     		kind:          domain.Autok,
//     		releaseClaims: releaseUlesek,
//     		summary:       autokSummary,
//     	})
//     }
//
//     Each use case starts with requireKiegeszitoOn, then calls
//     requireSetupAllowed for setup, or requireClaimAllowed (claiming for
//     oneself) or requireClaimAllowedBy(..., true) (the Szervezo moving
//     people, #17 and #18) for claims, and ends every write with
//     a.kiegeszitoChanged(esemenyID). Composable helpers: isResztvevo,
//     canClaim, ErrNotPermitted for "not yours". Kiegeszito errors live in
//     its file.
//  4. HTTP: httpapi/<kind>.go with register<Kind>(mux), one line in New in
//     httpapi.go. Map your own errors with writeCodedError and a code
//     prefixed with the kind ("autok_full"), then fall back to
//     writeKiegeszitoError.
//  5. Web: lib/kiegeszito/<kind>.ts (server-only, callBackend, a cache()d
//     getter); app/esemenyek/[id]/kiegeszitok/<kind>/panel.tsx (an async
//     server component taking KiegeszitoPanelProps) and <kind>/actions.ts
//     (each action wrapped in runKiegeszitoChange); in
//     kiegeszitok/registry.tsx set Panel on your pre-filled entry and add
//     hibak for your prefixed codes.
//  6. Tests: core tests in app_test through the use cases; httptest tests
//     for the routes. export_test.go has VetoSwitchOff and SetSwitchable.
//
// # Koltsegek (#20) and Elszamolas (#22)
//
// Koltsegek doesn't fit requireSetupAllowed or requireClaimAllowed: a
// Kiadas is recorded after the Esemeny took place too. Its writes need a
// guard of its own (requireKiadasokOpen): the Esemeny is Veglegesitve or
// Lezajlott and no Elszamolas has been calculated yet. On top of it, a
// Resztvevo adds a Kiadas, while its payer, its creator or the Szervezo
// edits or deletes it, Resztvevo or not. It registers vetoSwitchOff (not while it has Kiadasok)
// and no releaseClaims (Kiadasok are never touched automatically). Its
// switchable also allows Lezajlott, so a Szervezo who forgot can switch it
// on after the trip. See app/koltsegek.go (requireKiadasWrite, and the
// kiadasokLocked seam #22 fills in).

// Errors of the Kiegeszito framework, shared by every Kiegeszito.
var (
	// ErrKiegeszitoOff means the Kiegeszito is switched off for the
	// Esemeny, so its data is hidden.
	ErrKiegeszitoOff = errors.New("the kiegeszito is switched off")
	// ErrKiegeszitoLocked means the Kiegeszito can't be switched on or off
	// in the Esemeny's state (by default Elmaradt and Lezajlott).
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

// kiegeszitoJob is work a hook queues for after the use case commits, such
// as a DM. It gets the App, never the transaction. The rules:
//
//   - Read everything it needs inside the transaction and capture plain
//     values (ids, names, texts), never t.q or t itself: the transaction
//     is over when the job runs.
//   - Use only the DM and Discord helpers (a.sendDropOutDM and the like,
//     a.refreshEsemenyMessages). Never call a use case: in sync mode that
//     deadlocks the Discord work queue.
//   - The queue is lossy (a restart drops it): never use it for anything
//     that must happen, such as creating a Spotify playlist.
type kiegeszitoJob func(ctx context.Context, a *App)

// runKiegeszitoJobs runs queued jobs, from inside an afterCommit job.
func (a *App) runKiegeszitoJobs(ctx context.Context, jobs []kiegeszitoJob) {
	for _, job := range jobs {
		job(ctx, a)
	}
}

// kiegeszito is how one Kiegeszito plugs into the framework. Only kind is
// required; a nil hook means the Kiegeszito doesn't take part in it.
type kiegeszito struct {
	kind domain.KiegeszitoKind

	// switchable overrides in which states of the Esemeny the Szervezo can
	// switch the Kiegeszito on or off; nil means Egyeztetes and
	// Veglegesitve. (Koltsegek also allows Lezajlott.)
	switchable func(e domain.Esemeny) bool

	// vetoSwitchOff runs in the transaction before the Kiegeszito is
	// switched off. An error keeps it on and is returned to the Szervezo;
	// wrap ErrKiegeszitoInUse with the reason (Koltsegek while it has
	// Kiadasok).
	vetoSwitchOff func(ctx context.Context, t *kiegeszitoTx) error

	// switchedOn runs in the transaction each time the Kiegeszito goes from
	// off to on, so it must be idempotent: Lejatszasi lista (#23) creates
	// its playlist only when none is stored yet. An external call (Spotify)
	// never goes in t.afterCommit, whose queue is lossy: store a durable
	// job row (the timed jobs of #9) in this transaction, and let that job
	// make the call and store the playlist id in a transaction of its own.
	switchedOn func(ctx context.Context, t *kiegeszitoTx) error

	// releaseClaims releases, in the transaction, every claim whose holder
	// release reports true, and returns what was freed in short Hungarian
	// fragments for the Szervezo ("2 db sátor"). It runs on drop-outs and
	// on a Tag leaving (only that Felhasznalo) and on re-confirmation
	// (everyone who isn't a Resztvevo), whether the Kiegeszito is on or off,
	// because claims follow participation. Only when t.enabled is true may
	// it queue DMs (a Sofor's passengers, #17) with t.afterCommit: a hidden
	// Kiegeszito changes silently. Koltsegek has none: Kiadasok are never
	// touched.
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
// transaction, except for summary), the Esemeny as the change leaves it,
// whether the hook's Kiegeszito is on, and a way to queue work for after
// the commit.
type kiegeszitoTx struct {
	a *App
	q store.Queries
	e domain.Esemeny
	// enabled tells whether the Kiegeszito whose hook runs is on. Only then
	// may a releaser queue DMs.
	enabled bool

	after      []kiegeszitoJob
	resztvevok []domain.Resztvevo
}

// afterCommit queues work to run once the use case commits. See
// kiegeszitoJob for the rules.
func (t *kiegeszitoTx) afterCommit(job kiegeszitoJob) {
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
		out = append(out, KiegeszitoAllapot{Kind: k.kind, Enabled: enabled[k.kind], Switchable: kiegeszitoSwitchable(e, k)})
	}
	return out, nil
}

// kiegeszitoSwitchable tells whether the Kiegeszito can be switched on or
// off in the Esemeny's state: its own switchable rule if it has one, or
// else Egyeztetes and Veglegesitve. Elmaradt and Lezajlott freeze them.
func kiegeszitoSwitchable(e domain.Esemeny, k kiegeszito) bool {
	if k.switchable != nil {
		return k.switchable(e)
	}
	return e.Allapot == domain.Egyeztetes || e.Allapot == domain.Veglegesitve
}

// SetKiegeszito switches one Kiegeszito of an Esemeny on or off. Only the
// Szervezo may: a non-Tag gets ErrNotFound, another Tag ErrNotSzervezo. An
// unknown (or not yet built) Kiegeszito gives ErrInvalidInput, a state it
// can't be switched in ErrKiegeszitoLocked, and a veto an error wrapping
// ErrKiegeszitoInUse. Switching to the state it is already in changes
// nothing.
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
		if !kiegeszitoSwitchable(e, k) {
			return ErrKiegeszitoLocked
		}
		on, err := q.EnabledKiegeszitok(ctx, e.ID)
		if err != nil {
			return err
		}
		if on[kind] == enabled {
			return nil
		}
		// Both hooks run while it is (still or already) on.
		t = &kiegeszitoTx{a: a, q: q, e: e, enabled: true}
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
// jobs. Every use case that changes a Kiegeszito calls it.
func (a *App) kiegeszitoChanged(esemenyID string, after ...kiegeszitoJob) {
	a.afterCommit(func(ctx context.Context) {
		a.runKiegeszitoJobs(ctx, after)
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
// Koltsegek doesn't use it (see the top of this file).
func requireSetupAllowed(e domain.Esemeny) error {
	if !setupAllowed(e) {
		return ErrSetupClosed
	}
	return nil
}

// requireClaimAllowed lets actorID make or change a claim for themselves or
// their name-only Vendeg: requireClaimAllowedBy without the Szervezo's
// leeway.
func requireClaimAllowed(ctx context.Context, q store.Queries, e domain.Esemeny, actorID string, holder domain.ClaimHolder) error {
	return requireClaimAllowedBy(ctx, q, e, actorID, holder, false)
}

// requireClaimAllowedBy lets actorID make, change or remove a claim for
// holder:
//
//   - only while the Esemeny is Veglegesitve (ErrClaimingClosed), so claims
//     are frozen while it is back in Egyeztetes;
//   - for themselves; or, with allowSzervezo, for anyone when the actor is
//     the Szervezo (moving people between Autok or Agyak, #17 and #18);
//     anyone else gets ErrNotPermitted;
//   - for a name-only Vendeg (#13), whose Tag claims for them; #13 checks
//     the Vendeg and their Tag here, and until then a Vendeg is
//     ErrNotFound;
//   - only if the holder is a Resztvevo (ErrNotResztvevo), whoever acts.
func requireClaimAllowedBy(ctx context.Context, q store.Queries, e domain.Esemeny, actorID string, holder domain.ClaimHolder, allowSzervezo bool) error {
	if e.Allapot != domain.Veglegesitve {
		return ErrClaimingClosed
	}
	if holder.VendegID != "" {
		return fmt.Errorf("%w: no such vendeg", ErrNotFound)
	}
	bySzervezo := allowSzervezo && actorID == e.SzervezoID
	if holder.FelhasznaloID != actorID && !bySzervezo {
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

// canClaim is requireClaimAllowed for oneself as a yes or no, for views.
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
// holders release picks, each with t.enabled set to whether that one is on.
// It returns what was freed in the Kiegeszitok that are on (a hidden one
// isn't worth mentioning) and the jobs the releasers queued.
func (a *App) releaseClaims(ctx context.Context, q store.Queries, e domain.Esemeny, release func(domain.ClaimHolder) bool) ([]string, []kiegeszitoJob, error) {
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
		t.enabled = enabled[k.kind]
		f, err := k.releaseClaims(ctx, t, release)
		if err != nil {
			return nil, nil, fmt.Errorf("release %s claims: %w", k.kind, err)
		}
		if t.enabled {
			freed = append(freed, f...)
		}
	}
	return freed, t.after, nil
}

// releaseDropOut releases every claim of a Felhasznalo who dropped from
// igen (SetVisszajelzes) or stopped being a Tag (deleteTag).
func (a *App) releaseDropOut(ctx context.Context, q store.Queries, e domain.Esemeny, felhasznaloID string) ([]string, []kiegeszitoJob, error) {
	holder := domain.ClaimHolder{FelhasznaloID: felhasznaloID}
	return a.releaseClaims(ctx, q, e, func(h domain.ClaimHolder) bool { return h == holder })
}

// releaseNonResztvevok drops the claims of everyone who isn't a Resztvevo
// of the Esemeny as confirmed (see ConfirmIdopont). e must already be
// Veglegesitve with its new confirmed Idopont.
func (a *App) releaseNonResztvevok(ctx context.Context, q store.Queries, e domain.Esemeny) ([]kiegeszitoJob, error) {
	vs, err := q.ListVisszajelzesekOfEsemeny(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	stays := map[domain.ClaimHolder]bool{}
	for _, r := range resztvevokOf(e, vs) {
		stays[domain.HolderOf(r)] = true
	}
	// What was freed is thrown away: a re-confirmation is the Szervezo's
	// own act, announced as such, and sends no drop-out DM.
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
	t := &kiegeszitoTx{a: a, q: q, e: e, enabled: true}
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
