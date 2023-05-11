// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/email"
)

type (
	emailNotification struct {
		en          *email.Parcel
		sn          *sentNotification
		displayName string
	}
	emailNotificationParams struct {
		DisplayName, Email, Language, UserID string
		IsPushDisabled                       bool
	}
)

func (r *repository) sendEmailNotification(ctx context.Context, en *emailNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}

	if err := r.insertSentNotification(ctx, en.sn); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", en.sn)
	}
	en.en.From.Email = "no-reply@ice.io"
	if en.en.From.Name = internationalizedEmailDisplayNames[en.sn.Language]; en.en.From.Name == "" {
		en.en.From.Name = internationalizedEmailDisplayNames["en"]
	}

	if err := r.emailClient.Send(ctx, en.en, email.Participant{Name: en.displayName, Email: en.sn.NotificationChannelValue}); err != nil {
		return multierror.Append(
			errors.Wrapf(err, "failed to send email notification:%#v, desired to be sent:%#v", en.en, en.sn),
			errors.Wrapf(r.deleteSentNotification(ctx, en.sn), "failed to delete SENT_NOTIFICATIONS as a rollback for %#v", en.sn),
		)
	}

	return nil
}

func (r *repository) getEmailNotificationParams( //nolint:funlen,revive // .
	ctx context.Context, domain NotificationDomain, userID string, onlyIfPushDisabled bool,
) (*emailNotificationParams, error) {
	if ctx.Err() != nil {
		return nil, errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	sql := fmt.Sprintf(`SELECT u.username AS display_name, 
							   (CASE WHEN (u.disabled_email_notification_domains IS NULL 
												OR NOT (u.disabled_email_notification_domains @> ARRAY['%[1]v','%[2]v'])
										  )
							    		THEN u.email 
							    		ELSE '' 
								END) AS email, 
							   u.language,
							   u.user_id,
							   ( ( u.disabled_push_notification_domains IS NOT NULL 
								   AND (
										'%[1]v' = ANY(u.disabled_push_notification_domains)
										OR 
										'%[2]v' = ANY(u.disabled_push_notification_domains)
									   )
								 ) 
								 OR
								 (  SELECT * 
									FROM (SELECT FALSE 
										  WHERE EXISTS (SELECT 1
														FROM device_metadata dm
														WHERE dm.user_id = $1
														LIMIT 1)
										  UNION ALL
										  SELECT TRUE 
										 ) t
									LIMIT 1
								 ) 	   
							   ) AS is_push_disabled
						FROM users u
						WHERE u.user_id = $1
						GROUP BY u.user_id`, domain, AllNotificationDomain)
	resp, err := storage.Get[emailNotificationParams](ctx, r.db, sql, userID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to select for emailNotificationParams for `%v`, userID:%v", domain, userID)
	}
	if resp.Email == "" || resp.DisplayName == "" || (onlyIfPushDisabled && !resp.IsPushDisabled) {
		return nil, nil //nolint:nilnil // .
	}
	resp.DisplayName = strings.ToUpper(resp.DisplayName[:1]) + resp.DisplayName[1:]

	return resp, nil
}
