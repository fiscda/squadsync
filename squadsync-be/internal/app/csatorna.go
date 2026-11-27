package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// ErrCsatornaTaken means the channel is already linked to another Csoport.
// A channel belongs to at most one Csoport.
var ErrCsatornaTaken = errors.New("channel already linked to another csoport")

// LinkCsatornaInput names the Discord channel to link to a Csoport.
type LinkCsatornaInput struct {
	CsoportID string
	GuildID   string
	ChannelID string
	// ChannelName is shown on the web. It may be empty if it isn't known.
	ChannelName string
}

// LinkedCsatorna is the outcome of linking a channel.
type LinkedCsatorna struct {
	Csoport  domain.Csoport
	Csatorna domain.Csatorna
}

// LinkCsatorna links a Discord channel to a Csoport. A Csoport has at most
// one linked channel, so linking again replaces the previous one. A channel
// belongs to at most one Csoport: one linked elsewhere gives
// ErrCsatornaTaken. Only an Admin may link: another Tag gets ErrNotAdmin,
// anyone else ErrNotFound.
func (a *App) LinkCsatorna(ctx context.Context, felhasznaloID string, in LinkCsatornaInput) (LinkedCsatorna, error) {
	in.GuildID = strings.TrimSpace(in.GuildID)
	in.ChannelID = strings.TrimSpace(in.ChannelID)
	if in.GuildID == "" || in.ChannelID == "" {
		return LinkedCsatorna{}, fmt.Errorf("%w: a csatorna needs a guild id and a channel id", ErrInvalidInput)
	}
	result := LinkedCsatorna{Csatorna: domain.Csatorna{
		CsoportID:   in.CsoportID,
		GuildID:     in.GuildID,
		ChannelID:   in.ChannelID,
		ChannelName: strings.TrimSpace(in.ChannelName),
		LinkedBy:    felhasznaloID,
		LinkedAt:    a.clock.Now(),
	}}
	err := a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireAdmin(ctx, q, in.CsoportID, felhasznaloID); err != nil {
			return err
		}
		owner, err := q.FindCsoportIDOfChannel(ctx, in.ChannelID)
		if err == nil && owner != in.CsoportID {
			return ErrCsatornaTaken
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if result.Csoport, err = q.FindCsoport(ctx, in.CsoportID); err != nil {
			return notFound(err)
		}
		if err := q.DeleteCsatorna(ctx, in.CsoportID); err != nil {
			return err
		}
		return q.InsertCsatorna(ctx, result.Csatorna)
	})
	if err != nil {
		return LinkedCsatorna{}, err
	}
	return result, nil
}

// UnlinkCsatorna removes the Csoport's linked channel. Only an Admin may do
// it: another Tag gets ErrNotAdmin, anyone else ErrNotFound. Unlinking when
// no channel is linked is fine.
func (a *App) UnlinkCsatorna(ctx context.Context, felhasznaloID, csoportID string) error {
	return a.store.InTx(ctx, func(q store.Queries) error {
		if err := requireAdmin(ctx, q, csoportID, felhasznaloID); err != nil {
			return err
		}
		return q.DeleteCsatorna(ctx, csoportID)
	})
}

// AdminCsoportokOf lists the Csoports where the Felhasznalo is an Admin, in
// the order they became a Tag of them. The bot offers these when linking a
// channel.
func (a *App) AdminCsoportokOf(ctx context.Context, felhasznaloID string) ([]domain.CsoportSummary, error) {
	all, err := a.store.ListCsoportokOf(ctx, felhasznaloID)
	if err != nil {
		return nil, err
	}
	var admin []domain.CsoportSummary
	for _, c := range all {
		if c.Admin {
			admin = append(admin, c)
		}
	}
	return admin, nil
}

// requireAdmin checks that the Felhasznalo is an Admin of the Csoport. A Tag
// without the role gets ErrNotAdmin; anyone else ErrNotFound, whether or not
// the Csoport exists.
func requireAdmin(ctx context.Context, q store.Queries, csoportID, felhasznaloID string) error {
	tag, err := requireTag(ctx, q, csoportID, felhasznaloID)
	if err != nil {
		return err
	}
	if !tag.Admin {
		return ErrNotAdmin
	}
	return nil
}
