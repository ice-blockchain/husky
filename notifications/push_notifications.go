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
	"github.com/ice-blockchain/wintr/connectors/storage"
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
		_msgpack               struct{} `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // To insert we need asArray
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
	sql := fmt.Sprintf(`SELECT GROUP_CONCAT(dm.push_notification_token) AS push_notification_tokens, 
							   u.language,
							   u.user_id
						FROM users u
							 LEFT JOIN device_metadata dm
									ON ( u.disabled_push_notification_domains IS NULL 
										OR (
											POSITION('%[1]v', u.disabled_push_notification_domains) == 0
								   			AND 
								   			POSITION('%[2]v', u.disabled_push_notification_domains) == 0
								   		   )
								   	   )
								   AND dm.user_id = u.user_id
								   AND dm.push_notification_token IS NOT NULL 
								   AND dm.push_notification_token != ''
						WHERE u.user_id = :user_id`, domain, AllNotificationDomain)
	params := make(map[string]any, 1)
	params["user_id"] = userID
	resp := make([]*pushNotificationTokens, 0, 1)
	if err := r.db.PrepareExecuteTyped(sql, params, &resp); err != nil {
		return nil, errors.Wrapf(err, "failed to select for push notification tokens for `%v`, params:%#v", domain, params)
	}
	if len(resp) == 0 {
		return nil, errors.Wrapf(ErrNotFound, "user not found")
	}
	if resp[0].PushNotificationTokens == nil || len(*resp[0].PushNotificationTokens) == 0 {
		return nil, nil //nolint:nilnil // .
	}

	return resp[0], nil
}

type (
	sentNotificationPK struct {
		//nolint:unused,revive,tagliatelle,nosnakecase // Because it is used by the msgpack library for marshalling/unmarshalling.
		_msgpack                 struct{}            `msgpack:",asArray"`
		UserID                   string              `json:"userId,omitempty" example:"edfd8c02-75e0-4687-9ac2-1ce4723865c4"`
		Uniqueness               string              `json:"uniqueness,omitempty" example:"anything"`
		NotificationType         NotificationType    `json:"notificationType,omitempty" example:"adoption_changed"`
		NotificationChannel      NotificationChannel `json:"notificationChannel,omitempty" example:"email"`
		NotificationChannelValue string              `json:"notificationChannelValue,omitempty" example:"jdoe@example.com"`
	}
	sentNotification struct {
		_msgpack struct{}   `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // To insert we need asArray
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
	if err := storage.CheckNoSQLDMLErr(r.db.InsertTyped("SENT_NOTIFICATIONS", pn.sn, &[]*sentNotification{})); err != nil {
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
			errors.Wrapf(storage.CheckNoSQLDMLErr(r.db.DeleteTyped("SENT_NOTIFICATIONS", "pk_unnamed_SENT_NOTIFICATIONS_1", &pn.sn.sentNotificationPK, &[]*sentNotification{})), //nolint:lll // .
				"failed to delete SENT_NOTIFICATIONS as a rollback for %#v", pn.sn),
		).ErrorOrNil()
	}

	return nil
}

func (r *repository) clearInvalidPushNotificationToken(ctx context.Context, userID string, token push.DeviceToken) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := `UPDATE device_metadata
			SET push_notification_token = null
			WHERE user_id = :user_id
			  AND push_notification_token = :push_notification_token`
	params := make(map[string]any, 1+1)
	params["user_id"] = userID
	params["push_notification_token"] = token

	return errors.Wrapf(storage.CheckSQLDMLErr(r.db.PrepareExecute(sql, params)), "failed to update push_notification_token to empty for params:%#v", params)
}

type (
	sentAnnouncementPK struct {
		//nolint:unused,revive,tagliatelle,nosnakecase // Because it is used by the msgpack library for marshalling/unmarshalling.
		_msgpack                 struct{}            `msgpack:",asArray"`
		Uniqueness               string              `json:"uniqueness,omitempty" example:"anything"`
		NotificationType         NotificationType    `json:"notificationType,omitempty" example:"adoption_changed"`
		NotificationChannel      NotificationChannel `json:"notificationChannel,omitempty" example:"email"`
		NotificationChannelValue string              `json:"notificationChannelValue,omitempty" example:"jdoe@example.com"`
	}
	sentAnnouncement struct {
		_msgpack struct{}   `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // To insert we need asArray
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
	if err := storage.CheckNoSQLDMLErr(r.db.InsertTyped("SENT_ANNOUNCEMENTS", bpn.sa, &[]*sentAnnouncement{})); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", bpn.sa)
	}
	if err := r.pushNotificationsClient.Broadcast(ctx, bpn.pn); err != nil {
		return multierror.Append( //nolint:wrapcheck // Not needed.
			errors.Wrapf(err, "failed to broadcast push notification:%#v, desired to be sent:%#v", bpn.pn, bpn.sa),
			errors.Wrapf(storage.CheckNoSQLDMLErr(r.db.DeleteTyped("SENT_ANNOUNCEMENTS", "pk_unnamed_SENT_ANNOUNCEMENTS_1", &bpn.sa.sentAnnouncementPK, &[]*sentAnnouncement{})), //nolint:lll // .
				"failed to delete SENT_ANNOUNCEMENTS as a rollback for %#v", bpn.sa),
		).ErrorOrNil()
	}

	return nil
}
