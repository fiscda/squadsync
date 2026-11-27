package app

import (
	"context"
	"errors"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// The lifecycle of an Esemeny: Egyeztetes → Veglegesitve → Lezajlott, or
// Elmaradt. Only the Szervezo confirms, reopens or cancels:
//
//   - ConfirmIdopont: Egyeztetes → Veglegesitve, with any of its Idopontok.
//   - ReopenEgyeztetes: Veglegesitve → Egyeztetes, keeping every
//     Visszajelzes.
//   - CancelEsemeny: Egyeztetes or Veglegesitve → Elmaradt, which is final.
//
// Each change is announced: in the Csatorna if the Csoport has one, or else
// by DM to every Tag but the Szervezo. The Esemeny's Discord messages are
// re-rendered, so their state and answer buttons follow.

// Headlines of the announcements.
const (
	veglegesitveHeadline   = "✅ Véglegesítve"
	ujraEgyeztetesHeadline = "↩️ Újra egyeztetés"
	elmaradtHeadline       = "❌ Elmaradt"
)

// ConfirmIdopont confirms one Idopont of an Esemeny in Egyeztetes, which
// becomes Veglegesitve. Any Idopont may be confirmed, not only the first
// placed. A non-Tag, or an Idopont of another Esemeny, gets ErrNotFound; a
// Tag other than the Szervezo gets ErrNotSzervezo; an Esemeny not in
// Egyeztetes gets ErrInvalidTransition (to change the confirmed Idopont,
// reopen Egyeztetes first).
func (a *App) ConfirmIdopont(ctx context.Context, felhasznaloID, esemenyID, idopontID string) error {
	var ann gateway.Announcement
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzervezo(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		i, err := q.FindIdopont(ctx, idopontID)
		if err != nil {
			return notFound(err)
		}
		if i.EsemenyID != e.ID {
			return ErrNotFound
		}
		if e.Allapot != domain.Egyeztetes {
			return ErrInvalidTransition
		}
		// #16 drops the claims of people who aren't Resztvevok of the
		// newly confirmed Idopont here, in this transaction.
		if err := q.UpdateEsemenyAllapot(ctx, e.ID, domain.Veglegesitve, i.ID); err != nil {
			return err
		}
		ann, err = a.announcement(ctx, q, e, veglegesitveHeadline, idopontLabel(i, a.clock.Now()))
		return err
	})
	if err != nil {
		return err
	}
	a.afterCommit(func(ctx context.Context) { a.announceAllapot(ctx, esemenyID, felhasznaloID, ann) })
	return nil
}

// ReopenEgyeztetes takes a Veglegesitve Esemeny back to Egyeztetes: the
// confirmed Idopont is cleared and every Visszajelzes is kept. The errors
// are those of ConfirmIdopont; any state but Veglegesitve gives
// ErrInvalidTransition.
func (a *App) ReopenEgyeztetes(ctx context.Context, felhasznaloID, esemenyID string) error {
	var ann gateway.Announcement
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzervezo(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if e.Allapot != domain.Veglegesitve {
			return ErrInvalidTransition
		}
		// #16 freezes claims while the Esemeny is back in Egyeztetes.
		if err := q.UpdateEsemenyAllapot(ctx, e.ID, domain.Egyeztetes, ""); err != nil {
			return err
		}
		ann, err = a.announcement(ctx, q, e, ujraEgyeztetesHeadline, "")
		return err
	})
	if err != nil {
		return err
	}
	a.afterCommit(func(ctx context.Context) { a.announceAllapot(ctx, esemenyID, felhasznaloID, ann) })
	return nil
}

// CancelEsemeny makes an Esemeny in Egyeztetes or Veglegesitve Elmaradt.
// That is final: nothing can be answered, and it can't be confirmed or
// reopened. A confirmed Idopont is kept as a record of what was planned.
// The errors are those of ConfirmIdopont; any other state gives
// ErrInvalidTransition.
func (a *App) CancelEsemeny(ctx context.Context, felhasznaloID, esemenyID string) error {
	var ann gateway.Announcement
	err := a.store.InTx(ctx, func(q store.Queries) error {
		e, err := requireSzervezo(ctx, q, esemenyID, felhasznaloID)
		if err != nil {
			return err
		}
		if e.Allapot != domain.Egyeztetes && e.Allapot != domain.Veglegesitve {
			return ErrInvalidTransition
		}
		// The stored confirmed Idopont stays, only as a record; nothing reads
		// it in Elmaradt (see confirmedIdopont).
		if err := q.UpdateEsemenyAllapot(ctx, e.ID, domain.Elmaradt, e.ConfirmedIdopontID); err != nil {
			return err
		}
		ann, err = a.announcement(ctx, q, e, elmaradtHeadline, "")
		return err
	})
	if err != nil {
		return err
	}
	a.afterCommit(func(ctx context.Context) { a.announceAllapot(ctx, esemenyID, felhasznaloID, ann) })
	return nil
}

// requireSzervezo returns the Esemeny if the Felhasznalo is its Szervezo.
// A non-Tag gets ErrNotFound, any other Tag ErrNotSzervezo.
func requireSzervezo(ctx context.Context, q store.Queries, esemenyID, felhasznaloID string) (domain.Esemeny, error) {
	e, err := requireEsemenyTag(ctx, q, esemenyID, felhasznaloID)
	if err != nil {
		return domain.Esemeny{}, err
	}
	if e.SzervezoID != felhasznaloID {
		return domain.Esemeny{}, ErrNotSzervezo
	}
	return e, nil
}

// announcement builds the announcement of a change of the Esemeny's state,
// as it is at the change. Its Link is filled in when it is sent.
func (a *App) announcement(ctx context.Context, q store.Queries, e domain.Esemeny, headline, detail string) (gateway.Announcement, error) {
	csoport, err := q.FindCsoport(ctx, e.CsoportID)
	if err != nil {
		return gateway.Announcement{}, err
	}
	return gateway.Announcement{Headline: headline, CsoportName: csoport.Name, Title: e.Title, Detail: detail}, nil
}

// announceAllapot re-renders the Esemeny's Discord messages, then announces
// the change of its state. With a Csatorna, it posts the announcement there,
// linked to the Esemeny's post in that channel (or its web page if there
// is none). Without one, every Tag but the actor gets it by DM.
func (a *App) announceAllapot(ctx context.Context, esemenyID, actorID string, ann gateway.Announcement) {
	a.refreshEsemenyMessages(ctx, esemenyID)

	q := a.store.Queries
	e, err := q.FindEsemeny(ctx, esemenyID)
	if err != nil {
		a.log.Error("announce esemeny allapot", "esemeny", esemenyID, "err", err)
		return
	}
	ann.Link = a.esemenyURL(e.ID)
	csatorna, err := q.FindCsatorna(ctx, e.CsoportID)
	switch {
	case err == nil:
		if ref, ok := a.channelPost(ctx, e.ID, csatorna.ChannelID); ok {
			ann.Link = messageLink(csatorna.GuildID, ref)
		}
		cctx, cancel := context.WithTimeout(ctx, discordCallTimeout)
		_, err := a.discord.PostAnnouncement(cctx, csatorna.ChannelID, ann)
		cancel()
		a.logDiscordError("post announcement", err, "esemeny", e.ID, "channel_id", csatorna.ChannelID)
	case errors.Is(err, store.ErrNotFound):
		tags, err := q.ListTags(ctx, e.CsoportID)
		if err != nil {
			a.log.Error("announce esemeny allapot", "esemeny", esemenyID, "err", err)
			return
		}
		for _, t := range tags {
			if t.FelhasznaloID != actorID {
				a.sendAnnouncementDM(ctx, e.CsoportID, t.FelhasznaloID, ann)
			}
		}
	default:
		a.log.Error("announce esemeny allapot", "esemeny", esemenyID, "err", err)
	}
}

// channelPost finds the Esemeny's post in the channel, if it has one there.
func (a *App) channelPost(ctx context.Context, esemenyID, channelID string) (gateway.MessageRef, bool) {
	refs, err := a.store.ListEsemenyDiscordMessages(ctx, esemenyID)
	if err != nil {
		a.log.Error("find esemeny channel post", "esemeny", esemenyID, "err", err)
		return gateway.MessageRef{}, false
	}
	for _, r := range refs {
		if r.Kind == domain.InCsatorna && r.ChannelID == channelID {
			return gateway.MessageRef{ChannelID: r.ChannelID, MessageID: r.MessageID}, true
		}
	}
	return gateway.MessageRef{}, false
}

// dropOut builds the Szervezo's DM about a Resztvevo who dropped from igen.
// It runs in the transaction of the change; #16 passes what the drop-out
// freed (seats, beds, Tetelek) and adds it here.
func (a *App) dropOut(ctx context.Context, q store.Queries, e domain.Esemeny, felhasznaloID string) (gateway.DropOut, error) {
	f, err := q.FindFelhasznalo(ctx, felhasznaloID)
	if err != nil {
		return gateway.DropOut{}, err
	}
	csoport, err := q.FindCsoport(ctx, e.CsoportID)
	if err != nil {
		return gateway.DropOut{}, err
	}
	return gateway.DropOut{Name: f.Name, CsoportName: csoport.Name, Title: e.Title, Link: a.esemenyURL(e.ID)}, nil
}
