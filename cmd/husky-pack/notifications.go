// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"
	"net/http"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/notifications"
	"github.com/ice-blockchain/wintr/server"
)

func (s *service) setupNotificationsRoutes(router *server.Router) {
	router.
		Group("v1w").
		POST("user-pings/:userId", server.RootHandler(s.PingUser)).
		PUT("notification-channels/:notificationChannel/toggles/:type", server.RootHandler(s.ToggleNotificationChannelDomain)).
		PUT("inapp-notifications-user-auth-token", server.RootHandler(s.GenerateInAppNotificationsUserAuthToken))
}

// PingUser godoc
//
//	@Schemes
//	@Description	Pings the user.
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header	string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Param			userId			path	string	true	"ID of the user to ping"
//	@Success		202				"accepted"
//	@Failure		400				{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401				{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403				{object}	server.ErrorResponse	"not allowed"
//	@Failure		404				{object}	server.ErrorResponse	"if user not found"
//	@Failure		409				{object}	server.ErrorResponse	"if already pinged and need to try later"
//	@Failure		422				{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500				{object}	server.ErrorResponse
//	@Failure		504				{object}	server.ErrorResponse	"if request times out"
//	@Router			/user-pings/{userId} [POST].
func (s *service) PingUser( //nolint:gocritic // .
	ctx context.Context,
	req *server.Request[PingUserArg, any],
) (*server.Response[any], *server.Response[server.ErrorResponse]) {
	if err := s.notificationsProcessor.PingUser(ctx, req.Data.UserID); err != nil {
		err = errors.Wrapf(err, "failed to ping user for %#v", req.Data)
		switch {
		case errors.Is(err, notifications.ErrNotFound):
			return nil, server.NotFound(err, userNotFoundErrorCode)
		case errors.Is(err, notifications.ErrDuplicate):
			return nil, server.Conflict(err, userAlreadyPingedErrorCode)
		case errors.Is(err, notifications.ErrPingingUserNotAllowed):
			return nil, server.Forbidden(err)
		default:
			return nil, server.Unexpected(err)
		}
	}

	return &server.Response[any]{Code: http.StatusAccepted}, nil
}

// ToggleNotificationChannelDomain godoc
//
//	@Schemes
//	@Description	Toggles the specific notification channel toggle type on/off.
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header	string										true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Param			request				body	ToggleNotificationChannelDomainRequestBody	true	"Request params"
//	@Param			notificationChannel	path	string										true	"name of the channel"		enums(push,email)
//	@Param			type				path	string										true	"the type of the toggle"	enums(disable_all,weekly_report,weekly_stats,achievements,promotions,news,micro_community,mining,daily_bonus,system)
//	@Success		200					"ok"
//	@Failure		400					{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401					{object}	server.ErrorResponse	"if not authorized"
//	@Failure		404					{object}	server.ErrorResponse	"if user not found"
//	@Failure		422					{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500					{object}	server.ErrorResponse
//	@Failure		504					{object}	server.ErrorResponse	"if request times out"
//	@Router			/notification-channels/{notificationChannel}/toggles/{type} [PUT].
func (s *service) ToggleNotificationChannelDomain( //nolint:gocritic // False negative.
	ctx context.Context,
	req *server.Request[ToggleNotificationChannelDomainRequestBody, any],
) (*server.Response[any], *server.Response[server.ErrorResponse]) {
	if err := req.Data.validate(); err != nil {
		return nil, server.UnprocessableEntity(errors.Wrap(err, "validations failed"), invalidPropertiesErrorCode)
	}
	if err := s.notificationsProcessor.ToggleNotificationChannelDomain(ctx, req.Data.NotificationChannel, req.Data.Type, *req.Data.Enabled, req.AuthenticatedUser.UserID); err != nil { //nolint:lll // .
		err = errors.Wrapf(err, "failed to ToggleNotificationChannelDomain for %#v, userID:%v", req.Data, req.AuthenticatedUser.UserID)
		switch {
		case errors.Is(err, notifications.ErrRelationNotFound):
			return nil, server.NotFound(err, userNotFoundErrorCode)
		default:
			return nil, server.Unexpected(err)
		}
	}

	return server.OK[any](), nil
}

func (arg *ToggleNotificationChannelDomainRequestBody) validate() error {
	all := notifications.AllNotificationDomains[arg.NotificationChannel]
	if len(all) == 0 {
		return errors.Errorf("invalid notificationChannel `%v`", arg.NotificationChannel)
	}
	for _, domain := range all {
		if domain == arg.Type {
			return nil
		}
	}

	return errors.Errorf("invalid type `%v`", arg.Type)
}

// GenerateInAppNotificationsUserAuthToken godoc
//
//	@Schemes
//	@Description	Generates a new token for the user to be used to connect to the inApp notifications stream on behalf of the user.
//	@Tags			Notifications
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string	true	"Insert your access token"	default(Bearer <Add access token here>)
//	@Success		200				{object}	notifications.InAppNotificationsUserAuthToken
//	@Failure		400				{object}	server.ErrorResponse	"if validations fail"
//	@Failure		401				{object}	server.ErrorResponse	"if not authorized"
//	@Failure		403				{object}	server.ErrorResponse	"if not allowed"
//	@Failure		422				{object}	server.ErrorResponse	"if syntax fails"
//	@Failure		500				{object}	server.ErrorResponse
//	@Failure		504				{object}	server.ErrorResponse	"if request times out"
//	@Router			/inapp-notifications-user-auth-token [PUT].
func (s *service) GenerateInAppNotificationsUserAuthToken( //nolint:gocritic // False negative.
	ctx context.Context,
	req *server.Request[GenerateInAppNotificationsUserAuthTokenArg, notifications.InAppNotificationsUserAuthToken],
) (*server.Response[notifications.InAppNotificationsUserAuthToken], *server.Response[server.ErrorResponse]) {
	token, err := s.notificationsProcessor.GenerateInAppNotificationsUserAuthToken(ctx, req.AuthenticatedUser.UserID)
	if err != nil {
		return nil, server.Forbidden(errors.Wrapf(err, "not allowed"))
	}

	return server.OK(token), nil
}
