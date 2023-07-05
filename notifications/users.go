// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
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
		if storage.IsErr(err, ErrNotFound) {
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

func (r *repository) ToggleNotificationChannelDomain( //nolint:funlen,gocognit,gocyclo,revive,cyclop // .
	ctx context.Context, channel NotificationChannel, domain NotificationDomain, enabled bool, userID string,
) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	usr, err := r.getUserByID(ctx, userID)
	if err != nil {
		if storage.IsErr(err, ErrNotFound) {
			err = ErrRelationNotFound
		}

		return errors.Wrapf(err, "failed to get user by id:%v ", userID)
	}
	fieldForUpdate := ""
	var valuesForUpdate *users.Enum[NotificationDomain]
	switch channel { //nolint:exhaustive // We don't care about the rest.
	case EmailNotificationChannel: //nolint:dupl // .
		fieldForUpdate = "disabled_email_notification_domains = $2"
		if enabled && domain != DisableAllNotificationDomain { //nolint:nestif // .
			if usr.DisabledEmailNotificationDomains != nil && len(*usr.DisabledEmailNotificationDomains) > 0 {
				for i, disabledDomain := range *usr.DisabledEmailNotificationDomains {
					if domain == disabledDomain {
						(*usr.DisabledEmailNotificationDomains)[i] = ""
						valuesForUpdate = usr.DisabledEmailNotificationDomains

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
						valuesForUpdate = usr.DisabledEmailNotificationDomains
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
					valuesForUpdate = usr.DisabledEmailNotificationDomains
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
				valuesForUpdate = usr.DisabledEmailNotificationDomains
			}
		}
	case PushNotificationChannel: //nolint:dupl // .
		fieldForUpdate = "disabled_push_notification_domains = $2"
		if enabled && domain != DisableAllNotificationDomain { //nolint:nestif // .
			if usr.DisabledPushNotificationDomains != nil && len(*usr.DisabledPushNotificationDomains) > 0 {
				for i, disabledDomain := range *usr.DisabledPushNotificationDomains {
					if domain == disabledDomain {
						(*usr.DisabledPushNotificationDomains)[i] = ""
						valuesForUpdate = usr.DisabledPushNotificationDomains

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
						valuesForUpdate = usr.DisabledPushNotificationDomains
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
					valuesForUpdate = usr.DisabledPushNotificationDomains
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
				valuesForUpdate = usr.DisabledPushNotificationDomains
			}
		}
	default:
		log.Panic(fmt.Sprintf("channel `%v` not supported", channel))
	}
	if valuesForUpdate == nil {
		return nil
	}
	disabledDomains := *(valuesForUpdate)
	sanitizedDisabledDomains := make(users.Enum[NotificationDomain], 0, len(disabledDomains))
	for _, notificationDomain := range disabledDomains {
		if notificationDomain != "" {
			sanitizedDisabledDomains = append(sanitizedDisabledDomains, notificationDomain)
		}
	}
	valuesForUpdate = &sanitizedDisabledDomains
	sql := fmt.Sprintf(`UPDATE users SET %v where user_id = $1`, fieldForUpdate)
	if rowsUpdated, tErr := storage.Exec(ctx, r.db, sql, append([]any{userID}, valuesForUpdate)...); rowsUpdated == 0 || tErr != nil {
		if rowsUpdated == 0 && tErr == nil {
			tErr = ErrRelationNotFound
		}

		return errors.Wrapf(tErr, "failed to update users for userID:%v,ops:%#v with values %#v", userID, fieldForUpdate, valuesForUpdate)
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

	return errors.Wrapf(s.sendNewReferralNotification(ctx, snapshot), "failed to sendNewReferralNotification for :%#v", snapshot)
}

func (s *userTableSource) upsertUser(ctx context.Context, us *users.UserSnapshot) error { //nolint:funlen // Big SQL.
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	sql := `INSERT INTO USERS (
                   PHONE_NUMBER,
                   EMAIL,
                   FIRST_NAME,
                   LAST_NAME, 
                   USERNAME,
                   PROFILE_PICTURE_NAME,
                   REFERRED_BY,
                   PHONE_NUMBER_HASH,
                   LANGUAGE,
                   USER_ID
    ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
    ON CONFLICT(user_id)
      DO UPDATE
      	SET        PHONE_NUMBER = EXCLUDED.PHONE_NUMBER,
                   EMAIL = EXCLUDED.EMAIL,
                   FIRST_NAME = EXCLUDED.FIRST_NAME,
                   LAST_NAME = EXCLUDED.LAST_NAME,
                   USERNAME = EXCLUDED.USERNAME,
                   PROFILE_PICTURE_NAME = EXCLUDED.PROFILE_PICTURE_NAME,
                   REFERRED_BY = EXCLUDED.REFERRED_BY,
                   PHONE_NUMBER_HASH = EXCLUDED.PHONE_NUMBER_HASH,
                   LANGUAGE = EXCLUDED.LANGUAGE`
	_, err := storage.Exec(ctx, s.db, sql,
		us.PhoneNumber,
		us.Email,
		us.FirstName,
		us.LastName,
		us.Username,
		s.pictureClient.StripDownloadURL(strings.Replace(us.ProfilePictureURL, "profile/", "", 1)),
		us.ReferredBy,
		us.PhoneNumberHash,
		us.Language,
		us.ID,
	)

	return errors.Wrapf(err, "failed to upsert %#v", us)
}

func (s *userTableSource) deleteUser(ctx context.Context, us *users.UserSnapshot) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "context failed")
	}
	sql := `DELETE FROM users WHERE user_id = $1`
	if _, err := storage.Exec(ctx, s.db, sql, us.Before.ID); err != nil {
		return errors.Wrapf(err, "failed to delete user:%#v", us)
	}

	return errors.Wrapf(s.deleteDeviceMetadata(ctx, us.Before.ID), "failed to delete user:%#v", us)
}

func (s *userTableSource) deleteDeviceMetadata(ctx context.Context, userID string) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "[deleteDeviceMetadata] context failed")
	}
	sql := `DELETE FROM device_metadata WHERE user_id = $1`
	_, err := storage.Exec(ctx, s.db, sql, userID)

	return errors.Wrapf(err, "failed to delete device metadata for userID:%v", userID)
}

func (r *repository) getUserByID(ctx context.Context, userID string) (*user, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "context failed")
	}
	usr, err := storage.Get[user](ctx, r.db, `SELECT * FROM users WHERE user_id = $1`, userID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user by id: %#v", userID)
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
	sql := `INSERT INTO device_metadata (USER_ID, DEVICE_UNIQUE_ID, PUSH_NOTIFICATION_TOKEN) VALUES ($1, $2, $3)
			ON CONFLICT(USER_ID, DEVICE_UNIQUE_ID) DO UPDATE
			SET PUSH_NOTIFICATION_TOKEN = EXCLUDED.PUSH_NOTIFICATION_TOKEN`
	params := []any{
		snapshot.ID.UserID,
		snapshot.ID.DeviceUniqueID,
		snapshot.PushNotificationToken,
	}
	_, err := storage.Exec(ctx, u.db, sql, params...)

	return errors.Wrapf(err, "failed to upsert %#v", params...)
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
	var lastPingedTime *stdlibtime.Time
	if usr.LastPingCooldownEndedAt != nil {
		lastPingedTime = usr.LastPingCooldownEndedAt.Time
	}
	sql := `UPDATE users 
			   SET last_ping_cooldown_ended_at = $1
		    WHERE user_id = $2
			  AND referred_by = $3
			  AND COALESCE(last_ping_cooldown_ended_at, to_timestamp(0)) = COALESCE($4, to_timestamp(0))`
	params := []any{
		newPingCooldownEndsAt.Time,
		userID,
		usr.ReferredBy,
		lastPingedTime,
	}
	if rowsUpdated, sErr := storage.Exec(ctx, r.db, sql, params...); rowsUpdated == 0 && sErr == nil {
		return r.PingUser(ctx, userID)
	} else if sErr != nil {
		return errors.Wrapf(sErr, "failed to update users to set last_ping_cooldown_ended_at params:%#v", params...)
	}
	up := &UserPing{UserID: userID, PingedBy: reqUserID, LastPingCooldownEndedAt: newPingCooldownEndsAt}

	if err = r.sendUserPingMessage(ctx, up); err != nil {
		params[0] = usr.LastPingCooldownEndedAt
		params[3] = newPingCooldownEndsAt
		rRowsUpdated, rErr := storage.Exec(ctx, r.db, sql, params...)
		if rRowsUpdated == 0 && rErr == nil {
			return r.PingUser(ctx, userID)
		}

		return multierror.Append( //nolint:wrapcheck // Not needed.
			errors.Wrapf(err, "failed to sendUserPingMessage %#v", up),
			errors.Wrapf(rErr, "[rollback] failed to update users to set last_ping_cooldown_ended_at params:%#v", params...),
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
