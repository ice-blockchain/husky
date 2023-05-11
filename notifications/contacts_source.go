// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"

	"github.com/goccy/go-json"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func (s *agendaContactsSource) Process(ctx context.Context, msg *messagebroker.Message) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	contact := new(users.Contact)
	if err := json.UnmarshalContext(ctx, msg.Value, contact); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), contact)
	}
	if err := s.appendContactUserID(ctx, contact); err != nil {
		return errors.Wrapf(err, "can't upsert contacts for userID:%v", msg.Key)
	}

	return errors.Wrapf(s.sendNewAgendaContactNotification(ctx, contact), "failed to sendNewContactNotification for:%#v", contact.ContactUserID)
}

func (r *repository) appendContactUserID(ctx context.Context, contact *users.Contact) error {
	before, err := r.getAgendaContacts(ctx, contact.UserID)
	if err != nil && !storage.IsErr(err, storage.ErrNotFound) {
		return errors.Wrapf(err, "can't get contacts for userID:%v", contact.UserID)
	}
	for _, id := range before {
		if id == contact.ContactUserID {
			return ErrDuplicate
		}
	}
	sql := `UPDATE users SET agenda_contact_user_ids = array_append(agenda_contact_user_ids, $1)
				WHERE user_id = $2`
	_, err = storage.Exec(ctx, r.db, sql, contact.ContactUserID, contact.UserID)

	return errors.Wrapf(err, "can't append contactUserID:%#v for userID:%v", contact.ContactUserID, contact.UserID)
}

func (r *repository) getAgendaContacts(ctx context.Context, userID string) ([]string, error) {
	type contacts struct {
		AgendaContactUserIDs []string `db:"agenda_contact_user_ids"`
	}
	sql := `SELECT COALESCE(agenda_contact_user_ids,'{}'::TEXT[]) as agenda_contact_user_ids FROM users WHERE user_id = $1`
	res, err := storage.Get[contacts](ctx, r.db, sql, userID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get contact user ids for userID:%v", userID)
	}

	return res.AgendaContactUserIDs, nil
}
