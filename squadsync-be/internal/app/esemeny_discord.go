package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/gateway"
	"squadsync-be/internal/store"
)

// An Esemeny lives in Discord as one message: posted in the Csoport's
// Csatorna, or, without one, sent to each Tag by DM. The core decides what
// it says (gateway.EsemenyMessage); the bot lays it out. Every change to the
// Esemeny's counts or Helyezes edits each copy in place.
//
// Discord is called only after the use case's transaction commits, each
// call bounded by discordCallTimeout. A failure is logged and never fails
// the use case: proposing and answering work while Discord is down.

// discordCallTimeout bounds one call to Discord.
const discordCallTimeout = 10 * time.Second

// Notes under the Idopontok of an Esemeny's message.
const (
	tooManyIdopontok = "Több mint %d időpont van, ezért itt nem lehet gombokkal válaszolni: válaszolj a weboldalon."
	webPageNote      = "Weboldal: %s"
)

var allapotLabels = map[domain.Allapot]string{
	domain.Egyeztetes:   "Egyeztetés",
	domain.Veglegesitve: "Véglegesítve",
	domain.Elmaradt:     "Elmaradt",
	domain.Lezajlott:    "Lezajlott",
}

// esemenyMessage builds the Esemeny's Discord message.
func (a *App) esemenyMessage(ctx context.Context, q store.Queries, e domain.Esemeny) (gateway.EsemenyMessage, error) {
	d, err := esemenyDetail(ctx, q, e, "")
	if err != nil {
		return gateway.EsemenyMessage{}, err
	}
	pageURL := a.esemenyURL(e.ID)
	m := gateway.EsemenyMessage{
		EsemenyID:    e.ID,
		Title:        e.Title,
		Description:  e.Description,
		CsoportName:  d.CsoportName,
		Allapot:      allapotLabels[e.Allapot],
		SzervezoName: d.SzervezoName,
		HelyszinName: e.Helyszin.Name,
		HelyszinURL:  e.Helyszin.URL,
		Idopontok:    make([]gateway.IdopontLine, len(d.Idopontok)),
	}
	now := a.clock.Now()
	answerableCount := 0
	for n, allas := range d.Idopontok {
		line := gateway.IdopontLine{
			ID:         allas.ID,
			Label:      idopontLabel(allas.Idopont, now),
			Igen:       allas.Igen,
			Talan:      allas.Talan,
			Nem:        allas.Nem,
			Helyezes:   int(allas.Helyezes),
			Answerable: allas.Answerable,
			Confirmed:  allas.Confirmed,
		}
		if line.Answerable {
			answerableCount++
		}
		m.Idopontok[n] = line
	}
	for _, r := range d.Resztvevok {
		m.Resztvevok = append(m.Resztvevok, r.Name)
	}
	linkable := discordLinkable(pageURL)
	if linkable {
		m.URL = pageURL
	}
	var notes []string
	if answerableCount > gateway.MaxAnswerRows {
		// The buttons don't fit on one message: answer on the web.
		for n := range m.Idopontok {
			m.Idopontok[n].Answerable = false
		}
		notes = append(notes, fmt.Sprintf(tooManyIdopontok, gateway.MaxAnswerRows))
		m.WebButton = linkable
	}
	if !linkable {
		notes = append(notes, fmt.Sprintf(webPageNote, pageURL))
	}
	m.Note = strings.Join(notes, "\n")
	if m.KiegeszitoSummary, err = a.kiegeszitoSummary(ctx, q, e); err != nil {
		return gateway.EsemenyMessage{}, err
	}
	return m, nil
}

// discordLinkable tells whether Discord accepts u as an embed or button
// link: an http(s) URL whose host is a domain name with a dot. Discord
// rejects localhost, IP addresses and other hosts without a TLD, as when
// developing locally.
func discordLinkable(u string) bool {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "http" && p.Scheme != "https") {
		return false
	}
	host := p.Hostname()
	return strings.Contains(host, ".") && net.ParseIP(host) == nil && host != "localhost"
}

func (a *App) esemenyURL(esemenyID string) string {
	return strings.TrimRight(a.webURL, "/") + "/esemenyek/" + esemenyID
}

// announceEsemeny announces a newly proposed Esemeny. With a Csatorna, it
// posts the Esemeny's message there and asks every Tag but the Szervezo, by
// DM, to answer. Without one, every Tag gets the Esemeny's message by DM,
// the Szervezo too, so they have something in Discord to follow.
func (a *App) announceEsemeny(ctx context.Context, esemenyID string) {
	q := a.store.Queries
	e, err := q.FindEsemeny(ctx, esemenyID)
	if err != nil {
		a.log.Error("announce esemeny", "esemeny", esemenyID, "err", err)
		return
	}
	m, err := a.esemenyMessage(ctx, q, e)
	if err != nil {
		a.log.Error("announce esemeny", "esemeny", esemenyID, "err", err)
		return
	}
	tags, err := q.ListTags(ctx, e.CsoportID)
	if err != nil {
		a.log.Error("announce esemeny", "esemeny", esemenyID, "err", err)
		return
	}
	csatorna, err := q.FindCsatorna(ctx, e.CsoportID)
	hasCsatorna := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		a.log.Error("announce esemeny", "esemeny", esemenyID, "err", err)
		return
	}

	voteRequest := gateway.VoteRequest{CsoportName: m.CsoportName, Title: m.Title}
	if !hasCsatorna {
		for _, t := range tags {
			for _, ref := range a.sendEsemenyDM(ctx, e.CsoportID, t.FelhasznaloID, voteRequest, m) {
				a.rememberMessage(ctx, e.ID, domain.InDM, ref)
			}
		}
		return
	}

	cctx, cancel := context.WithTimeout(ctx, discordCallTimeout)
	ref, err := a.discord.PostEsemenyMessage(cctx, csatorna.ChannelID, m)
	cancel()
	a.logDiscordError("post esemeny", err, "esemeny", e.ID, "channel_id", csatorna.ChannelID)
	if err == nil {
		a.rememberMessage(ctx, e.ID, domain.InCsatorna, ref)
		voteRequest.ChannelMessageURL = messageLink(csatorna.GuildID, ref)
	}
	voteRequest.WebURL = a.esemenyURL(e.ID)
	for _, t := range tags {
		if t.FelhasznaloID == e.SzervezoID {
			continue // the Szervezo has just proposed it
		}
		a.sendVoteRequestDM(ctx, e.CsoportID, t.FelhasznaloID, voteRequest)
	}
}

// refreshEsemenyMessages edits every Discord message of the Esemeny to show
// its current state. It is the only writer of those messages after they
// are sent: a button press acknowledges without changing the message and
// leaves the update to this job, so edits never race.
func (a *App) refreshEsemenyMessages(ctx context.Context, esemenyID string) {
	q := a.store.Queries
	refs, err := q.ListEsemenyDiscordMessages(ctx, esemenyID)
	if err != nil || len(refs) == 0 {
		if err != nil {
			a.log.Error("refresh esemeny messages", "esemeny", esemenyID, "err", err)
		}
		return
	}
	e, err := q.FindEsemeny(ctx, esemenyID)
	if err != nil {
		a.log.Error("refresh esemeny messages", "esemeny", esemenyID, "err", err)
		return
	}
	m, err := a.esemenyMessage(ctx, q, e)
	if err != nil {
		a.log.Error("refresh esemeny messages", "esemeny", esemenyID, "err", err)
		return
	}
	for _, r := range refs {
		ref := gateway.MessageRef{ChannelID: r.ChannelID, MessageID: r.MessageID}
		cctx, cancel := context.WithTimeout(ctx, discordCallTimeout)
		err := a.discord.EditEsemenyMessage(cctx, ref, m)
		cancel()
		a.logDiscordError("edit esemeny message", err, "esemeny", esemenyID, "channel_id", ref.ChannelID, "message_id", ref.MessageID)
	}
}

// rememberMessage stores a posted message so later changes can edit it.
func (a *App) rememberMessage(ctx context.Context, esemenyID string, kind domain.EsemenyMessageKind, ref gateway.MessageRef) {
	err := a.store.InsertEsemenyDiscordMessage(ctx, domain.EsemenyDiscordMessage{
		EsemenyID: esemenyID,
		Kind:      kind,
		ChannelID: ref.ChannelID,
		MessageID: ref.MessageID,
		CreatedAt: a.clock.Now(),
	})
	if err != nil {
		a.log.Error("remember esemeny message", "esemeny", esemenyID, "message_id", ref.MessageID, "err", err)
	}
}

// messageLink is the URL that opens a message in a server's channel.
func messageLink(guildID string, ref gateway.MessageRef) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", guildID, ref.ChannelID, ref.MessageID)
}

var (
	honapok = [...]string{"jan.", "febr.", "márc.", "ápr.", "máj.", "jún.", "júl.", "aug.", "szept.", "okt.", "nov.", "dec."}
	napok   = [...]string{"vasárnap", "hétfő", "kedd", "szerda", "csütörtök", "péntek", "szombat"}
)

// idopontLabel shows an Idopont in Budapest time, e.g.
// "dec. 5. (szombat) 20:00–23:00", or "aug. 14. (péntek) – aug. 16.
// (vasárnap)" for a whole-day one. The year is shown only when it isn't the
// current one.
func idopontLabel(i domain.Idopont, now time.Time) string {
	thisYear := now.In(Budapest).Year()
	start := i.Start.In(Budapest)
	if i.EgeszNapos && i.End != nil {
		// End is midnight after the last day.
		last := i.End.In(Budapest).Add(-time.Minute)
		if sameDay(start, last) {
			return dayLabel(start, thisYear) + ", egész nap"
		}
		return dayLabel(start, thisYear) + " – " + dayLabel(last, thisYear)
	}
	label := dayLabel(start, thisYear) + " " + start.Format("15:04")
	if i.End == nil {
		return label
	}
	end := i.End.In(Budapest)
	if sameDay(start, end) {
		return label + "–" + end.Format("15:04")
	}
	return label + " – " + dayLabel(end, thisYear) + " " + end.Format("15:04")
}

// dayLabel shows a Budapest day, e.g. "dec. 5. (szombat)", with the year
// in front when it isn't thisYear.
func dayLabel(t time.Time, thisYear int) string {
	label := fmt.Sprintf("%s %d. (%s)", honapok[t.Month()-1], t.Day(), napok[t.Weekday()])
	if t.Year() != thisYear {
		label = fmt.Sprintf("%d. %s", t.Year(), label)
	}
	return label
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
