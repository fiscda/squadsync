package app

import (
	"context"

	"squadsync-be/internal/domain"
	"squadsync-be/internal/store"
)

// A Veglegesitve Esemeny has timed jobs tied to its confirmed Idopont: the
// Lezajlott transition and the Emlekeztetok. Confirming schedules them;
// reopening Egyeztetes and cancelling cancel them. Each job also checks, when
// it runs, that the Esemeny is still Veglegesitve on the same Idopont, so a
// stale one does nothing.

// lezajlottPayload identifies a Lezajlott job: the confirmed Idopont it
// belongs to.
type lezajlottPayload struct {
	IdopontID string `json:"idopont"`
}

// scheduleVeglegesitve schedules the jobs of a newly confirmed Idopont, in
// the confirming transaction: Lezajlott at its end (or its start, if it has
// no end), and the Emlekeztetok before its start.
func (a *App) scheduleVeglegesitve(ctx context.Context, q store.Queries, e domain.Esemeny, i domain.Idopont) error {
	at := i.Start
	if i.End != nil {
		at = *i.End
	}
	if err := a.scheduleJob(ctx, q, jobLezajlott, e.ID, at, lezajlottPayload{IdopontID: i.ID}); err != nil {
		return err
	}
	return a.scheduleEmlekeztetok(ctx, q, e, i, true)
}

// cancelVeglegesitve cancels the pending jobs of the confirmed Idopont, in
// the transaction that reopens Egyeztetes or cancels the Esemeny.
func cancelVeglegesitve(ctx context.Context, q store.Queries, esemenyID string) error {
	return cancelJobs(ctx, q, esemenyID, jobLezajlott, jobEmlekezteto)
}

// runLezajlott makes the Esemeny Lezajlott, if it is still Veglegesitve on
// the job's Idopont, and re-renders its Discord messages (state label, no
// answer buttons).
func (a *App) runLezajlott(ctx context.Context, q store.Queries, j store.Job) (func(context.Context), error) {
	var p lezajlottPayload
	if err := jobPayload(j, &p); err != nil {
		return nil, err
	}
	e, err := q.FindEsemeny(ctx, j.EsemenyID)
	if err != nil {
		return nil, err
	}
	if e.Allapot != domain.Veglegesitve || e.ConfirmedIdopontID != p.IdopontID {
		return nil, nil
	}
	if err := q.UpdateEsemenyAllapot(ctx, e.ID, domain.Lezajlott, p.IdopontID); err != nil {
		return nil, err
	}
	// #22 schedules the Elszamolas reminder here, 3 days after j.RunAt (when
	// the Esemeny really ended, even if this job ran late).
	return func(ctx context.Context) { a.refreshEsemenyMessages(ctx, e.ID) }, nil
}
