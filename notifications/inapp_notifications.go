// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"

	storagev2 "github.com/ice-blockchain/wintr/connectors/storage/v2"

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

func (r *repository) sendInAppNotification(ctx context.Context, in *inAppNotification) error { //nolint:dupl // Wrong.
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	return storagev2.DoInTransaction(ctx, r.db, func(conn storagev2.QueryExecer) error {
		if err := r.insertSentNotification(ctx, conn, in.sn); err != nil {
			return errors.Wrapf(err, "failed to insert %#v", in.sn)
		}
		return errors.Wrapf(r.personalInAppFeed.Send(ctx, in.in, in.sn.UserID),
			"failed to send inApp notification:%#v, desired to be sent:%#v", in.in, in.sn)
	})
}

func (r *repository) broadcastInAppNotification(ctx context.Context, bin *broadcastInAppNotification) error { //nolint:dupl // Wrong.
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	return storagev2.DoInTransaction(ctx, r.db, func(conn storagev2.QueryExecer) error {
		if err := r.insertSentAnnouncement(ctx, conn, bin.sa); err != nil {
			return errors.Wrapf(err, "failed to insert %#v", bin.sa)
		}

		return errors.Wrapf(r.globalInAppFeed.Send(ctx, bin.in, bin.sa.NotificationChannelValue),
			"failed to broadcast inApp notification:%#v, desired to be sent:%#v", bin.in, bin.sa)
	})
}

func (r *repository) GenerateInAppNotificationsUserAuthToken(ctx context.Context, userID string) (*InAppNotificationsUserAuthToken, error) {
	if true {
		return &InAppNotificationsUserAuthToken{}, nil
	}

	return r.personalInAppFeed.CreateUserToken(ctx, userID) //nolint:wrapcheck // No need, we can just proxy it.
}
