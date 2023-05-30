// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/notifications/inapp"
)

type (
	inAppNotification struct {
		in *inapp.Parcel
		sn *sentNotification
	}
	broadcastInAppNotification struct {
		in *inapp.Parcel
		sa *sentAnnouncement
	}
)

func (r *repository) sendInAppNotification(ctx context.Context, in *inAppNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if true {
		return nil
	}

	if err := r.insertSentNotification(ctx, in.sn); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", in.sn)
	}

	if err := r.personalInAppFeed.Send(ctx, in.in, in.sn.UserID); err != nil {
		return multierror.Append( //nolint:wrapcheck // .
			errors.Wrapf(err, "failed to send inApp notification:%#v, desired to be sent:%#v", in.in, in.sn),
			errors.Wrapf(r.deleteSentNotification(ctx, in.sn), "failed to delete SENT_NOTIFICATIONS as a rollback for %#v", in.sn),
		).ErrorOrNil()
	}

	return nil
}

func (r *repository) broadcastInAppNotification(ctx context.Context, bin *broadcastInAppNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if true {
		return nil
	}

	if err := r.insertSentAnnouncement(ctx, bin.sa); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", bin.sa)
	}

	if err := r.globalInAppFeed.Send(ctx, bin.in, bin.sa.NotificationChannelValue); err != nil {
		return multierror.Append( //nolint:wrapcheck // .
			errors.Wrapf(err, "failed to broadcast inApp notification:%#v, desired to be sent:%#v", bin.in, bin.sa),
			errors.Wrapf(r.deleteSentAnnouncement(ctx, bin.sa), "failed to delete SENT_ANNOUNCEMENTS as a rollback for %#v", bin.sa),
		).ErrorOrNil()
	}

	return nil
}

func (r *repository) GenerateInAppNotificationsUserAuthToken(ctx context.Context, userID string) (*InAppNotificationsUserAuthToken, error) {
	if true {
		return &InAppNotificationsUserAuthToken{}, nil
	}

	return r.personalInAppFeed.CreateUserToken(ctx, userID) //nolint:wrapcheck // No need, we can just proxy it.
}
