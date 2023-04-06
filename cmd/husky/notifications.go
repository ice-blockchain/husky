// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/notifications"
	"github.com/ice-blockchain/wintr/server"
)

func (s *service) setupNotificationsRoutes(router *server.Router) {
	router.
		Group("v1r").
		GET("notification-channels/:notificationChannel/toggles", server.RootHandler(s.GetNotificationChannelToggles))
}

// GetNotificationChannelToggles godoc
//
//	@Schemes
//	@Description	Returns the user's list of notification channel toggles for the provided notificationChannel.
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Param			notificationChannel	path		string	true	"email/push"				enums(push,email)
//	@Success		200					{array}		notifications.NotificationChannelToggle
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/notification-channels/{notificationChannel}/toggles [GET].
func (s *service) GetNotificationChannelToggles( //nolint:gocritic // False negative.
	ctx context.Context,
	req *server.Request[GetNotificationChannelTogglesArg, []*notifications.NotificationChannelToggle],
) (*server.Response[[]*notifications.NotificationChannelToggle], *server.Response[server.ErrorResponse]) {
	if req.Data.NotificationChannel != notifications.PushNotificationChannel && req.Data.NotificationChannel != notifications.EmailNotificationChannel {
		return nil, server.UnprocessableEntity(errors.Errorf("invalid notificationChannel `%v`", req.Data.NotificationChannel), invalidPropertiesErrorCode)
	}
	resp, err := s.notificationsRepository.GetNotificationChannelToggles(ctx, req.Data.NotificationChannel, req.AuthenticatedUser.UserID)
	if err != nil {
		return nil, server.Unexpected(errors.Wrapf(err, "failed to GetNotificationChannelToggles for %#v, userID:%v", req.Data, req.AuthenticatedUser.UserID))
	}

	return server.OK(&resp), nil
}
