package backend

import (
	"context"
	"errors"

	"github.com/GabiHert/deputy/internal/app"
	"github.com/GabiHert/deputy/internal/update"
)

var errNoUpdater = errors.New("Updates are not available in this build.")

func updateDTO(s update.Status) app.UpdateStatusDTO {
	d := app.UpdateStatusDTO{Version: s.Version, Repo: s.Repo, UpToDate: s.UpToDate, HasToken: s.HasToken,
		State: s.State, Message: s.Message, Lines: s.Lines, Seq: s.Seq, InstalledApp: s.InstalledApp}
	if r := s.Latest; r != nil {
		d.Latest = &app.UpdateReleaseDTO{Tag: r.Tag, Name: r.Name, Notes: r.Notes, Date: ms(r.Date), URL: r.URL}
	}
	return d
}

func (b *Backend) GetUpdateStatus() app.UpdateStatusDTO {
	if b.d.Update == nil {
		return app.UpdateStatusDTO{Version: b.d.Version, State: "idle", Lines: []string{}}
	}
	return updateDTO(b.d.Update.Status())
}

func (b *Backend) CheckForUpdate(ctx context.Context) (app.UpdateStatusDTO, error) {
	if b.d.Update == nil {
		return b.GetUpdateStatus(), errNoUpdater
	}
	s, err := b.d.Update.Check(ctx)
	return updateDTO(s), err
}

func (b *Backend) StartUpdate() error {
	if b.d.Update == nil {
		return errNoUpdater
	}
	return b.d.Update.Start()
}

func (b *Backend) SetUpdateToken(ctx context.Context, token string) (app.UpdateStatusDTO, error) {
	if b.d.Update == nil {
		return b.GetUpdateStatus(), errNoUpdater
	}
	if err := b.d.Update.SetToken(ctx, token); err != nil {
		return b.GetUpdateStatus(), err
	}
	return b.GetUpdateStatus(), nil
}
