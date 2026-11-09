package backend

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/domain"
)

var memberCache sync.Map // chat jid -> membersEntry

type membersEntry struct {
	at   time.Time
	list []app.MemberDTO
}

// GroupMembers lists a group's participants for @mentions, cached for five minutes.
func (b *Backend) GroupMembers(ctx context.Context, chat string) ([]app.MemberDTO, error) {
	if domain.IsDirect(chat) {
		return nil, errors.New("only groups have members")
	}
	if e, ok := memberCache.Load(chat); ok && time.Since(e.(membersEntry).at) < 5*time.Minute {
		return e.(membersEntry).list, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ms, err := b.d.WA.GroupMembers(ctx, chat)
	if err != nil {
		return nil, err
	}
	out := make([]app.MemberDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, app.MemberDTO{JID: m.JID, Name: m.Name, Phone: m.Phone, Users: m.Users, Me: m.Me, Admin: m.Admin})
	}
	memberCache.Store(chat, membersEntry{at: time.Now(), list: out})
	return out, nil
}
