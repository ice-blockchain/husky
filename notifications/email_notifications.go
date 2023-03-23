// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/connectors/storage"
	"github.com/ice-blockchain/wintr/email"
)

type (
	emailNotification struct {
		en          *email.Parcel
		sn          *sentNotification
		displayName string
	}
	emailNotificationParams struct {
		_msgpack                             struct{} `msgpack:",asArray"` //nolint:unused,tagliatelle,revive,nosnakecase // To insert we need asArray
		DisplayName, Email, Language, UserID string
		IsPushDisabled                       bool
	}
)

func (r *repository) sendEmailNotification(ctx context.Context, en *emailNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if err := storage.CheckNoSQLDMLErr(r.db.InsertTyped("SENT_NOTIFICATIONS", en.sn, &[]*sentNotification{})); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", en.sn)
	}

	en.en.From.Email = "no-reply@ice.io"
	if en.en.From.Name = internationalizedEmailDisplayNames[en.sn.Language]; en.en.From.Name == "" {
		en.en.From.Name = internationalizedEmailDisplayNames["en"]
	}
	if err := r.emailClient.Send(ctx, en.en, email.Participant{Name: en.displayName, Email: en.sn.NotificationChannelValue}); err != nil {
		return multierror.Append( //nolint:wrapcheck // Not needed.
			errors.Wrapf(err, "failed to send email notification:%#v, desired to be sent:%#v", en.en, en.sn),
			errors.Wrapf(storage.CheckNoSQLDMLErr(r.db.DeleteTyped("SENT_NOTIFICATIONS", "pk_unnamed_SENT_NOTIFICATIONS_1", &en.sn.sentNotificationPK, &[]*sentNotification{})), //nolint:lll // .
				"failed to delete SENT_NOTIFICATIONS as a rollback for %#v", en.sn),
		).ErrorOrNil()
	}

	return nil
}

func (r *repository) getEmailNotificationParams( //nolint:funlen,revive // .
	ctx context.Context, domain NotificationDomain, userID string, onlyIfPushDisabled bool,
) (*emailNotificationParams, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := fmt.Sprintf(`SELECT u.username, 
							   (CASE WHEN (u.disabled_email_notification_domains IS NULL 
												OR (
													POSITION('%[1]v', u.disabled_email_notification_domains) == 0
													AND 
													POSITION('%[2]v', u.disabled_email_notification_domains) == 0
												   ))
							    		THEN u.email 
							    		ELSE '' 
								END) AS email, 
							   u.language,
							   u.user_id,
							   ( ( u.disabled_push_notification_domains IS NOT NULL 
								   AND (
										POSITION('%[1]v', u.disabled_push_notification_domains) > 0
										OR 
										POSITION('%[2]v', u.disabled_push_notification_domains) > 0
									   )
								 ) 
								 OR
								 (  SELECT * 
									FROM (SELECT FALSE 
										  WHERE EXISTS (SELECT 1
														FROM device_metadata dm
														WHERE dm.user_id = :user_id
														LIMIT 1)
										  UNION ALL
										  SELECT TRUE 
										 )
									LIMIT 1
								 ) 	   
							   ) AS is_push_disabled
						FROM users u
						WHERE u.user_id = :user_id`, domain, AllNotificationDomain)
	params := make(map[string]any, 1)
	params["user_id"] = userID
	resp := make([]*emailNotificationParams, 0, 1)
	if err := r.db.PrepareExecuteTyped(sql, params, &resp); err != nil {
		return nil, errors.Wrapf(err, "failed to select for emailNotificationParams for `%v`, params:%#v", domain, params)
	}
	if len(resp) == 0 {
		return nil, errors.Wrapf(ErrNotFound, "user not found")
	}
	if resp[0].Email == "" || resp[0].DisplayName == "" || (onlyIfPushDisabled && !resp[0].IsPushDisabled) {
		return nil, nil //nolint:nilnil // .
	}
	resp[0].DisplayName = strings.ToUpper(resp[0].DisplayName[:1]) + resp[0].DisplayName[1:]

	return resp[0], nil
}
