// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/go-tarantool-client"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/time"
)

func (r *repository) GetNotificationChannelToggles( //nolint:funlen,gocognit,gocyclo,revive,cyclop // .
	ctx context.Context, channel NotificationChannel, userID string,
) (resp []*NotificationChannelToggle, err error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	resp = r.defaultNotificationChannelToggles(channel)
	usr, err := r.getUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return resp, nil
		}

		return nil, errors.Wrapf(err, "failed to get user by id:%v ", userID)
	}
	switch channel { //nolint:exhaustive // We don't care about the rest.
	case EmailNotificationChannel:
		if usr.DisabledEmailNotificationDomains != nil && len(*usr.DisabledEmailNotificationDomains) > 0 {
			for _, domain := range *usr.DisabledEmailNotificationDomains {
				for _, toggle := range resp {
					if toggle.Type == DisableAllNotificationDomain && domain == AllNotificationDomain {
						toggle.Enabled = true
					} else if domain == toggle.Type {
						toggle.Enabled = false
					}
				}
			}
		}
	case PushNotificationChannel:
		if usr.DisabledPushNotificationDomains != nil && len(*usr.DisabledPushNotificationDomains) > 0 {
			for _, domain := range *usr.DisabledPushNotificationDomains {
				for _, toggle := range resp {
					if toggle.Type == DisableAllNotificationDomain && domain == AllNotificationDomain {
						toggle.Enabled = true
					} else if domain == toggle.Type {
						toggle.Enabled = false
					}
				}
			}
		}
	default:
		log.Panic(fmt.Sprintf("channel `%v` not supported", channel))
	}

	return resp, nil
}

func (*repository) defaultNotificationChannelToggles(channel NotificationChannel) []*NotificationChannelToggle {
	all := AllNotificationDomains[channel]
	resp := make([]*NotificationChannelToggle, 0, len(all))
	for _, domain := range all {
		resp = append(resp, &NotificationChannelToggle{
			Type:    domain,
			Enabled: domain != DisableAllNotificationDomain,
		})
	}

	return resp
}

func (r *repository) ToggleNotificationChannelDomain( //nolint:funlen,gocognit,gocyclo,revive,cyclop,maintidx // .
	ctx context.Context, channel NotificationChannel, domain NotificationDomain, enabled bool, userID string,
) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	usr, err := r.getUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			err = ErrRelationNotFound
		}

		return errors.Wrapf(err, "failed to get user by id:%v ", userID)
	}
	op := tarantool.Op{Op: "="}
	switch channel { //nolint:exhaustive // We don't care about the rest.
	case EmailNotificationChannel: //nolint:dupl // .
		op.Field = 2
		if enabled && domain != DisableAllNotificationDomain { //nolint:nestif // .
			if usr.DisabledEmailNotificationDomains != nil && len(*usr.DisabledEmailNotificationDomains) > 0 {
				for i, disabledDomain := range *usr.DisabledEmailNotificationDomains {
					if domain == disabledDomain {
						(*usr.DisabledEmailNotificationDomains)[i] = ""
						op.Arg = usr.DisabledEmailNotificationDomains

						break
					}
				}
			}
		} else {
			if usr.DisabledEmailNotificationDomains != nil && len(*usr.DisabledEmailNotificationDomains) > 0 {
				alreadyThere := !enabled && domain == DisableAllNotificationDomain
				for i, disabledDomain := range *usr.DisabledEmailNotificationDomains {
					if !enabled && domain == DisableAllNotificationDomain && disabledDomain == AllNotificationDomain {
						(*usr.DisabledEmailNotificationDomains)[i] = ""
						op.Arg = usr.DisabledEmailNotificationDomains
						alreadyThere = true

						break
					}
					if (enabled && domain == DisableAllNotificationDomain && disabledDomain == AllNotificationDomain) || domain == disabledDomain {
						alreadyThere = true

						break
					}
				}
				if !alreadyThere {
					actualDomain := domain
					if domain == DisableAllNotificationDomain {
						actualDomain = AllNotificationDomain
					}
					*usr.DisabledEmailNotificationDomains = append(*usr.DisabledEmailNotificationDomains, actualDomain)
					op.Arg = usr.DisabledEmailNotificationDomains
				}
			} else {
				if !enabled && domain == DisableAllNotificationDomain {
					break
				}
				actualDomain := domain
				if domain == DisableAllNotificationDomain {
					actualDomain = AllNotificationDomain
				}
				disabledDomains := append(make(users.Enum[NotificationDomain], 0, 1), actualDomain)
				usr.DisabledEmailNotificationDomains = &disabledDomains
				op.Arg = usr.DisabledEmailNotificationDomains
			}
		}
	case PushNotificationChannel: //nolint:dupl // .
		op.Field = 1
		if enabled && domain != DisableAllNotificationDomain { //nolint:nestif // .
			if usr.DisabledPushNotificationDomains != nil && len(*usr.DisabledPushNotificationDomains) > 0 {
				for i, disabledDomain := range *usr.DisabledPushNotificationDomains {
					if domain == disabledDomain {
						(*usr.DisabledPushNotificationDomains)[i] = ""
						op.Arg = usr.DisabledPushNotificationDomains

						break
					}
				}
			}
		} else {
			if usr.DisabledPushNotificationDomains != nil && len(*usr.DisabledPushNotificationDomains) > 0 {
				alreadyThere := !enabled && domain == DisableAllNotificationDomain
				for i, disabledDomain := range *usr.DisabledPushNotificationDomains {
					if !enabled && domain == DisableAllNotificationDomain && disabledDomain == AllNotificationDomain {
						(*usr.DisabledPushNotificationDomains)[i] = ""
						op.Arg = usr.DisabledPushNotificationDomains
						alreadyThere = true

						break
					}
					if (enabled && domain == DisableAllNotificationDomain && disabledDomain == AllNotificationDomain) || domain == disabledDomain {
						alreadyThere = true

						break
					}
				}
				if !alreadyThere {
					actualDomain := domain
					if domain == DisableAllNotificationDomain {
						actualDomain = AllNotificationDomain
					}
					*usr.DisabledPushNotificationDomains = append(*usr.DisabledPushNotificationDomains, actualDomain)
					op.Arg = usr.DisabledPushNotificationDomains
				}
			} else {
				if !enabled && domain == DisableAllNotificationDomain {
					break
				}
				actualDomain := domain
				if domain == DisableAllNotificationDomain {
					actualDomain = AllNotificationDomain
				}
				disabledDomains := append(make(users.Enum[NotificationDomain], 0, 1), actualDomain)
				usr.DisabledPushNotificationDomains = &disabledDomains
				op.Arg = usr.DisabledPushNotificationDomains
			}
		}
	default:
		log.Panic(fmt.Sprintf("channel `%v` not supported", channel))
	}
	if op.Arg == nil {
		return nil
	}
	disabledDomains := *(op.Arg.(*users.Enum[NotificationDomain])) //nolint:forcetypeassert // We know for sure.
	sanitizedDisabledDomains := make(users.Enum[NotificationDomain], 0, len(disabledDomains))
	for _, notificationDomain := range disabledDomains {
		if notificationDomain != "" {
			sanitizedDisabledDomains = append(sanitizedDisabledDomains, notificationDomain)
		}
	}
	op.Arg = &sanitizedDisabledDomains
	resp := make([]*user, 0, 1)
	if err = storage.CheckNoSQLDMLErr(r.db.UpdateTyped("USERS", "pk_unnamed_USERS_1", tarantool.StringKey{S: userID}, append(make([]tarantool.Op, 0, 1), op), &resp)); err != nil { //nolint:lll // .
		if errors.Is(err, storage.ErrNotFound) {
			err = ErrRelationNotFound
		}

		return errors.Wrapf(err, "failed to update users for userID:%v,ops:%#v", userID, op)
	}
	if len(resp) == 0 || resp[0].UserID == "" { //nolint:revive // Wrong.
		return ErrRelationNotFound
	}

	return nil
}

func (s *userTableSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:gocognit // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	snapshot := new(users.UserSnapshot)
	if err := json.UnmarshalContext(ctx, msg.Value, snapshot); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), snapshot)
	}
	if (snapshot.Before == nil || snapshot.Before.ID == "") && (snapshot.User == nil || snapshot.User.ID == "") {
		return nil
	}
	if snapshot.Before != nil && snapshot.Before.ID != "" && (snapshot.User == nil || snapshot.User.ID == "") {
		return errors.Wrapf(s.deleteUser(ctx, snapshot), "failed to delete user:%#v", snapshot)
	}
	if err := s.upsertUser(ctx, snapshot); err != nil {
		return errors.Wrapf(err, "failed to upsert:%#v", snapshot)
	}

	return multierror.Append(nil, //nolint:wrapcheck // Not needed.
		errors.Wrapf(s.sendNewReferralNotification(ctx, snapshot), "failed to sendNewReferralNotification for :%#v", snapshot),
		errors.Wrapf(s.sendNewContactNotification(ctx, snapshot), "failed to sendNewContactNotification for :%#v", snapshot),
	).ErrorOrNil()
}

func (s *userTableSource) upsertUser(ctx context.Context, us *users.UserSnapshot) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	insertTuple := &user{
		PhoneNumber:             us.PhoneNumber,
		Email:                   us.Email,
		FirstName:               us.FirstName,
		LastName:                us.LastName,
		UserID:                  us.ID,
		Username:                us.Username,
		ProfilePictureName:      s.pictureClient.StripDownloadURL(strings.Replace(us.ProfilePictureURL, "profile/", "", 1)),
		ReferredBy:              us.ReferredBy,
		PhoneNumberHash:         us.PhoneNumberHash,
		AgendaPhoneNumberHashes: us.AgendaPhoneNumberHashes,
		Language:                us.Language,
	}
	//nolint:gomnd // Those are the field indices.
	ops := append(make([]tarantool.Op, 0, 10),
		tarantool.Op{Op: "=", Field: 4, Arg: us.PhoneNumber},
		tarantool.Op{Op: "=", Field: 5, Arg: us.Email},
		tarantool.Op{Op: "=", Field: 6, Arg: us.FirstName},
		tarantool.Op{Op: "=", Field: 7, Arg: us.LastName},
		tarantool.Op{Op: "=", Field: 9, Arg: us.Username},
		tarantool.Op{Op: "=", Field: 10, Arg: s.pictureClient.StripDownloadURL(strings.Replace(us.ProfilePictureURL, "profile/", "", 1))},
		tarantool.Op{Op: "=", Field: 11, Arg: us.ReferredBy},
		tarantool.Op{Op: "=", Field: 12, Arg: us.PhoneNumberHash},
		tarantool.Op{Op: "=", Field: 13, Arg: us.AgendaPhoneNumberHashes},
		tarantool.Op{Op: "=", Field: 14, Arg: us.Language})

	return errors.Wrapf(storage.CheckNoSQLDMLErr(s.db.UpsertTyped("USERS", insertTuple, ops, &[]*user{})), "failed to upsert %#v", us)
}

func (s *userTableSource) deleteUser(ctx context.Context, us *users.UserSnapshot) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	sql := `DELETE FROM users WHERE user_id = :user_id`
	params := map[string]any{"user_id": us.Before.ID}

	return errors.Wrapf(storage.CheckSQLDMLErr(s.db.PrepareExecute(sql, params)), "failed to delete user:%#v", us)
}

func (r *repository) getUserByID(ctx context.Context, userID string) (*user, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	usr := new(user)
	if err := r.db.GetTyped("USERS", "pk_unnamed_USERS_1", tarantool.StringKey{S: userID}, usr); err != nil {
		return nil, errors.Wrapf(err, "failed to get user by id: %#v", userID)
	}
	if usr.UserID == "" {
		return nil, storage.ErrNotFound
	}

	return usr, nil
}

func (u *deviceMetadataTableSource) Process(ctx context.Context, msg *messagebroker.Message) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	snapshot := new(users.DeviceMetadataSnapshot)
	if err := json.UnmarshalContext(ctx, msg.Value, snapshot); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), snapshot)
	}
	if snapshot.DeviceMetadata == nil || snapshot.DeviceMetadata.ID.UserID == "" ||
		(snapshot.Before != nil && snapshot.Before.PushNotificationToken == snapshot.DeviceMetadata.PushNotificationToken) {
		return nil
	}
	type deviceMetadata struct {
		_msgpack struct{} `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // .
		users.DeviceID
		PushNotificationToken string
	}
	insertTuple := deviceMetadata{
		DeviceID:              snapshot.ID,
		PushNotificationToken: snapshot.PushNotificationToken,
	}
	ops := append(make([]tarantool.Op, 0, 1), tarantool.Op{Op: "=", Field: 2, Arg: snapshot.PushNotificationToken}) //nolint:gomnd // Field index.

	return errors.Wrapf(storage.CheckNoSQLDMLErr(u.db.UpsertTyped("DEVICE_METADATA", insertTuple, ops, &[]*deviceMetadata{})),
		"failed to upsert %#v", insertTuple)
}

func (r *repository) PingUser(ctx context.Context, userID string) error { //nolint:funlen,gocognit,revive // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected context deadline")
	}
	usr, err := r.getUserByID(ctx, userID)
	if err != nil {
		return errors.Wrapf(err, "failed to getUserByID for userID:%v", userID)
	}
	reqUserID := requestingUserID(ctx)
	if usr.ReferredBy != reqUserID {
		ref, refErr := r.getUserByID(ctx, reqUserID)
		if refErr != nil {
			return errors.Wrapf(refErr, "failed to getUserByID for reqUserID:%v", reqUserID)
		}
		if ref.ReferredBy != userID {
			return ErrPingingUserNotAllowed
		}
	}
	now := time.Now()
	newPingCooldownEndsAt := time.New(now.Add(r.cfg.PingCooldown))
	if usr.LastPingCooldownEndedAt != nil && usr.LastPingCooldownEndedAt.After(*now.Time) {
		return ErrDuplicate
	}
	sql := `UPDATE users 
			   SET last_ping_cooldown_ended_at = :now_nanos
		    WHERE user_id = :user_id
			  AND referred_by = :referred_by
			  AND IFNULL(last_ping_cooldown_ended_at, 0) = IFNULL(:last_ping_cooldown_ended_at, 0)`
	params := make(map[string]any, 1+1+1+1)
	params["user_id"] = userID
	params["referred_by"] = usr.ReferredBy
	params["now_nanos"] = newPingCooldownEndsAt
	params["last_ping_cooldown_ended_at"] = usr.LastPingCooldownEndedAt
	if err = storage.CheckSQLDMLErr(r.db.PrepareExecute(sql, params)); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return r.PingUser(ctx, userID)
		}

		return errors.Wrapf(err, "failed to update users to set last_ping_cooldown_ended_at params:%#v", params)
	}
	up := &UserPing{UserID: userID, PingedBy: reqUserID, LastPingCooldownEndedAt: newPingCooldownEndsAt}

	if err = r.sendUserPingMessage(ctx, up); err != nil {
		params["now_nanos"] = usr.LastPingCooldownEndedAt
		params["last_ping_cooldown_ended_at"] = newPingCooldownEndsAt
		rErr := storage.CheckSQLDMLErr(r.db.PrepareExecute(sql, params))
		if rErr != nil && errors.Is(rErr, storage.ErrNotFound) {
			return r.PingUser(ctx, userID)
		}

		return multierror.Append( //nolint:wrapcheck // Not needed.
			errors.Wrapf(err, "failed to sendUserPingMessage %#v", up),
			errors.Wrapf(rErr, "[rollback] failed to update users to set last_ping_cooldown_ended_at params:%#v", params),
		).ErrorOrNil()
	}

	return nil
}

func (r *repository) sendUserPingMessage(ctx context.Context, up *UserPing) error {
	valueBytes, err := json.MarshalContext(ctx, up)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %#v", up)
	}
	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "husky"},
		Key:     up.UserID,
		Topic:   r.cfg.MessageBroker.Topics[1].Name,
		Value:   valueBytes,
	}
	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send `%v` message to broker", msg.Topic)
}
