// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"strings"

	"github.com/goccy/go-json"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
)

func (s *contactsTableSource) Process(ctx context.Context, msg *messagebroker.Message) error {
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
	if err := s.upsertContacts(ctx, contact); err != nil {
		return errors.Wrapf(err, "can't upsert contacts for userID:%v", msg.Key)
	}

	return errors.Wrapf(s.sendNewContactNotification(ctx, contact), "failed to sendNewContactNotification for:%#v", contact.ContactUserID)
}

func (r *repository) upsertContacts(ctx context.Context, contact *users.Contact) error {
	before, err := r.getContacts(ctx, contact.UserID)
	if err != nil && !storage.IsErr(err, storage.ErrNotFound) {
		return errors.Wrapf(err, "can't get contacts for userID:%v", contact.UserID)
	}
	var toUpsert []string
	if before != nil {
		ids := strings.Split(before.ContactUserIDs, ",")
		for _, id := range ids {
			if id == contact.ContactUserID {
				return nil
			}
		}
		toUpsert = append(toUpsert, ids...)
	}
	toUpsert = append(toUpsert, contact.ContactUserID)
	sql := `INSERT INTO contacts(user_id, contact_user_ids) VALUES ($1, $2)
				ON CONFLICT(user_id)
				DO UPDATE
					SET contact_user_ids = EXCLUDED.contact_user_ids
				WHERE contacts.contact_user_ids != EXCLUDED.contact_user_ids`
	_, err = storage.Exec(ctx, r.db, sql, contact.UserID, strings.Join(toUpsert, ","))

	return errors.Wrapf(err, "can't insert/update contact user id:%#v for userID:%v", toUpsert, contact.UserID)
}

func (r *repository) getContacts(ctx context.Context, userID string) (*contacts, error) {
	sql := `SELECT * FROM contacts WHERE user_id = $1`
	res, err := storage.Get[contacts](ctx, r.db, sql, userID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get contact user ids for userID:%v", userID)
	}

	return res, nil
}
