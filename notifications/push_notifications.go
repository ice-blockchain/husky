// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

type (
	pushNotificationTemplate struct {
		title, body *template.Template
		Title       string `json:"title"` //nolint:revive // That's intended.
		Body        string `json:"body"`  //nolint:revive // That's intended.
	}
)

func (t *pushNotificationTemplate) getTitle(data any) string {
	if data == nil {
		return t.Title
	}
	bf := new(bytes.Buffer)
	log.Panic(errors.Wrapf(t.title.Execute(bf, data), "failed to execute title template for data:%#v", data))

	return bf.String()
}

func (t *pushNotificationTemplate) getBody(data any) string {
	if data == nil {
		return t.Body
	}
	bf := new(bytes.Buffer)
	log.Panic(errors.Wrapf(t.body.Execute(bf, data), "failed to execute body template for data:%#v", data))

	return bf.String()
}

func loadPushNotificationTranslationTemplates() {
	const totalLanguages = 50
	allPushNotificationTemplates = make(map[NotificationType]map[languageCode]*pushNotificationTemplate, len(AllNotificationTypes))
	for _, notificationType := range AllNotificationTypes {
		files, err := translations.ReadDir(fmt.Sprintf("translations/push/%v", notificationType))
		if err != nil {
			panic(err)
		}
		allPushNotificationTemplates[notificationType] = make(map[languageCode]*pushNotificationTemplate, totalLanguages)
		for _, file := range files {
			content, fErr := translations.ReadFile(fmt.Sprintf("translations/push/%v/%v", notificationType, file.Name()))
			if fErr != nil {
				panic(fErr)
			}
			var tmpl pushNotificationTemplate
			err = json.Unmarshal(content, &tmpl)
			if err != nil {
				panic(err)
			}
			language := strings.Split(file.Name(), ".")[0]
			tmpl.title = template.Must(template.New(fmt.Sprintf("push_%v_%v_title", notificationType, language)).Parse(tmpl.Title))
			tmpl.body = template.Must(template.New(fmt.Sprintf("push_%v_%v_body", notificationType, language)).Parse(tmpl.Body))
			allPushNotificationTemplates[notificationType][language] = &tmpl
		}
	}
}

type (
	pushNotificationTokens struct {
		PushNotificationTokens *users.Enum[push.DeviceToken]
		Language, UserID       string
		Postpone               bool
	}
)

func (r *repository) getPushNotificationTokens(
	ctx context.Context, domain NotificationDomain, userID string,
) (*pushNotificationTokens, error) {
	return r.getPushNotificationTokensOrPostpone(ctx, domain, "", userID)
}

//nolint:funlen // .
func (r *repository) getPushNotificationTokensOrPostpone(
	ctx context.Context, domain NotificationDomain, shouldPostponeSQL, userID string,
) (pnt *pushNotificationTokens, err error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	postponeFormatted := ""
	if len(shouldPostponeSQL) > 0 {
		postponeFormatted = fmt.Sprintf(", %v as postpone", shouldPostponeSQL)
	}
	sql := fmt.Sprintf(`SELECT array_agg(dm.push_notification_token) filter (where dm.push_notification_token is not null)  AS push_notification_tokens, 
							   u.language,
							   u.user_id
							   %[3]v
						FROM users u
							 LEFT JOIN device_metadata dm
									ON ( u.disabled_push_notification_domains IS NULL 
										OR NOT (u.disabled_push_notification_domains @> ARRAY['%[1]v','%[2]v'] )
								   	   )
								   AND dm.user_id = u.user_id
								   AND dm.push_notification_token IS NOT NULL 
								   AND dm.push_notification_token != ''
						WHERE u.user_id = $1
						GROUP BY u.user_id`, domain, AllNotificationDomain, postponeFormatted)
	resp, err := storage.Get[pushNotificationTokens](ctx, r.db, sql, userID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to select for push notification tokens for `%v`, userID:%#v", domain, userID)
	}
	if resp.PushNotificationTokens == nil || len(*resp.PushNotificationTokens) == 0 {
		return nil, nil //nolint:nilnil // .
	}

	return resp, nil
}

type (
	sentNotificationPK struct {
		UserID                   string              `json:"userId,omitempty" example:"edfd8c02-75e0-4687-9ac2-1ce4723865c4"`
		Uniqueness               string              `json:"uniqueness,omitempty" example:"anything"`
		NotificationType         NotificationType    `json:"notificationType,omitempty" example:"adoption_changed"`
		NotificationChannel      NotificationChannel `json:"notificationChannel,omitempty" example:"email"`
		NotificationChannelValue string              `json:"notificationChannelValue,omitempty" example:"jdoe@example.com"`
	}
	sentNotification struct {
		SentAt   *time.Time `json:"sentAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		Language string     `json:"language,omitempty" example:"en"`
		sentNotificationPK
	}
	pushNotification struct {
		pn       *push.Notification[push.DeviceToken]
		sn       *sentNotification
		postpone bool
	}
)

func (r *repository) sendPushNotification(ctx context.Context, pn *pushNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if pn.postpone {
		return r.postponePushNotification(ctx, pn)
	}
	if err := r.insertSentNotification(ctx, pn.sn); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", pn.sn)
	}
	responder := make(chan error, 1)
	defer close(responder)
	r.pushNotificationsClient.Send(ctx, pn.pn, responder)
	if err := <-responder; err != nil {
		var cErr error
		if errors.Is(err, push.ErrInvalidDeviceToken) {
			cErr = r.clearInvalidPushNotificationToken(ctx, pn.sn.UserID, pn.pn.Target)
		}

		return multierror.Append( //nolint:wrapcheck // Not needed.
			errors.Wrapf(cErr, "failed to clearInvalidPushNotificationToken for userID:%#v, push token:%#v", pn.sn.UserID, pn.pn.Target),
			errors.Wrapf(err, "failed to send push notification:%#v, desired to be sent:%#v", pn.pn, pn.sn),
			errors.Wrapf(r.deleteSentNotification(ctx, pn.sn), "failed to delete SENT_NOTIFICATIONS as a rollback for %#v", pn.sn),
		).ErrorOrNil()
	}

	return nil
}

func (r *repository) postponePushNotification(ctx context.Context, pn *pushNotification) error {
	serializedPN, err := json.MarshalContext(ctx, pn.pn)
	if err != nil {
		return errors.Wrapf(err, "failed to serialize %#v", pn.pn)
	}
	sql := `INSERT INTO postponed_notifications (
                                POSTPONED_AT,
                                LANGUAGE,
                                USER_ID,
                                UNIQUENESS,
                                NOTIFICATION_TYPE,
                                NOTIFICATION_CHANNEL,
                                NOTIFICATION_CHANNEL_VALUE,
                                NOTIFICATION_DATA
        	) VALUES ($1,$2,$3,$4,$5,$6,$7, $8);`

	_, err = storage.Exec(ctx, r.db, sql,
		pn.sn.SentAt.Time,
		pn.sn.Language,
		pn.sn.UserID,
		pn.sn.Uniqueness,
		pn.sn.NotificationType,
		pn.sn.NotificationChannel,
		pn.sn.NotificationChannelValue,
		serializedPN,
	)

	return errors.Wrapf(err, "failed to insert postponed notification %#v", pn)
}

func (r *repository) resendPostponedNotificationsForUserID(ctx context.Context, userID string) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "[resendPostponedNotificationsForUserID] unexpected deadline")
	}
	notifications, err := storage.ExecMany[postponedNotification](ctx, r.db, `DELETE FROM postponed_notifications WHERE user_id = $1 RETURNING *`, userID)
	if err != nil {
		return errors.Wrapf(err, "failed to get postponed notifications for userID:%v", userID)
	}
	pn := make([]*pushNotification, 0, len(notifications))
	for _, notification := range notifications {
		var decodedPN push.Notification[push.DeviceToken]
		if err = json.UnmarshalContext(ctx, []byte(notification.NotificationData), &decodedPN); err != nil {
			return errors.Wrapf(err, "failed to decerialize notification data for userID:%v %v", userID, notification.NotificationData)
		}
		pn = append(pn, &pushNotification{
			sn: &sentNotification{
				SentAt:             notification.PostponedAt,
				Language:           notification.Language,
				sentNotificationPK: notification.sentNotificationPK,
			},
			pn: &decodedPN,
		})
	}

	return errors.Wrapf(runConcurrently(ctx, func(ctx context.Context, n *pushNotification) error {
		if sErr := r.sendPushNotification(ctx, n); sErr != nil {
			return multierror.Append(sErr, r.postponePushNotification(ctx, n)).ErrorOrNil() //nolint:wrapcheck // .
		}

		return nil
	}, pn), "failed to send some of postponed notifications for userID:%v %#v", userID, pn)
}

func (r *repository) clearInvalidPushNotificationToken(ctx context.Context, userID string, token push.DeviceToken) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `UPDATE device_metadata
			SET push_notification_token = null
			WHERE user_id = $1
			  AND push_notification_token = $2`
	_, err := storage.Exec(ctx, r.db, sql, userID, token)

	return errors.Wrapf(err, "failed to update push_notification_token to empty for userID:%v and token %v", userID, token)
}

type (
	sentAnnouncementPK struct {
		Uniqueness               string              `json:"uniqueness,omitempty" example:"anything"`
		NotificationType         NotificationType    `json:"notificationType,omitempty" example:"adoption_changed"`
		NotificationChannel      NotificationChannel `json:"notificationChannel,omitempty" example:"email"`
		NotificationChannelValue string              `json:"notificationChannelValue,omitempty" example:"jdoe@example.com"`
	}
	sentAnnouncement struct {
		SentAt   *time.Time `json:"sentAt,omitempty" example:"2022-01-03T16:20:52.156534Z"`
		Language string     `json:"language,omitempty" example:"en"`
		sentAnnouncementPK
	}
	broadcastPushNotification struct {
		pn *push.Notification[push.SubscriptionTopic]
		sa *sentAnnouncement
	}
)

func (r *repository) broadcastPushNotification(ctx context.Context, bpn *broadcastPushNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}

	if err := r.insertSentAnnouncement(ctx, bpn.sa); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", bpn.sa)
	}

	if err := r.pushNotificationsClient.Broadcast(ctx, bpn.pn); err != nil {
		return multierror.Append( //nolint:wrapcheck // .
			errors.Wrapf(err, "failed to broadcast push notification:%#v, desired to be sent:%#v", bpn.pn, bpn.sa),
			errors.Wrapf(r.deleteSentAnnouncement(ctx, bpn.sa), "failed to delete SENT_ANNOUNCEMENTS as a rollback for %#v", bpn.sa),
		).ErrorOrNil()
	}

	return nil
}
