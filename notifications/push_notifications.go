// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"bytes"
	"context"
	"fmt"
	storagev2 "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"strings"
	"text/template"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
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
	}
)

func (r *repository) getPushNotificationTokens( //nolint:funlen // .
	ctx context.Context, domain NotificationDomain, userID string,
) (*pushNotificationTokens, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := fmt.Sprintf(`SELECT string_agg(dm.push_notification_token, ',') AS push_notification_tokens, 
							   u.language,
							   u.user_id
						FROM users u
							 LEFT JOIN device_metadata dm
									ON ( u.disabled_push_notification_domains IS NULL 
										OR (
											POSITION('%[1]v' IN u.disabled_push_notification_domains) = 0
								   			AND 
								   			POSITION('%[2]v' IN u.disabled_push_notification_domains) = 0
								   		   )
								   	   )
								   AND dm.user_id = u.user_id
								   AND dm.push_notification_token IS NOT NULL 
								   AND dm.push_notification_token != ''
						WHERE u.user_id = $1
						GROUP BY u.user_id`, domain, AllNotificationDomain)
	resp, err := storagev2.Get[pushNotificationTokens](ctx, r.db, sql, userID)
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
		pn *push.Notification[push.DeviceToken]
		sn *sentNotification
	}
)

func (r *repository) sendPushNotification(ctx context.Context, pn *pushNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	err := storagev2.DoInTransaction(ctx, r.db, func(conn storagev2.QueryExecer) error {
		if err := r.insertSentNotification(ctx, conn, pn.sn); err != nil {
			return errors.Wrapf(err, "failed to insert %#v", pn.sn)
		}
		responder := make(chan error, 1)
		defer close(responder)
		r.pushNotificationsClient.Send(ctx, pn.pn, responder)
		if err := <-responder; err != nil {
			if errors.Is(err, push.ErrInvalidDeviceToken) {
				return push.ErrInvalidDeviceToken
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, push.ErrInvalidDeviceToken) {
			return multierror.Append(err, r.clearInvalidPushNotificationToken(ctx, pn.sn.UserID, pn.pn.Target))
		}
	}
	return err
}

func (r *repository) clearInvalidPushNotificationToken(ctx context.Context, userID string, token push.DeviceToken) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `UPDATE device_metadata
			SET push_notification_token = null
			WHERE user_id = $1
			  AND push_notification_token = $2`
	_, err := storagev2.Exec(ctx, r.db, sql, userID, token)
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
	return storagev2.DoInTransaction(ctx, r.db, func(conn storagev2.QueryExecer) error {
		if err := r.insertSentAnnouncement(ctx, conn, bpn.sa); err != nil {
			return errors.Wrapf(err, "failed to insert %#v", bpn.sa)
		}
		return errors.Wrapf(r.pushNotificationsClient.Broadcast(ctx, bpn.pn), "failed to broadcast push notification:%#v, desired to be sent:%#v", bpn.pn, bpn.sa)
	})
}
