// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/notifications"
	"github.com/ice-blockchain/wintr/server"
)

func (s *service) setupUserNotificationsRoutes(router *gin.Engine) {
	router.
		Group("v1w").
		POST("notifications", server.RootHandler(newRequestNotifyUser, s.NotifyUser))
}

// NotifyUser godoc
// @Schemes
// @Description  Notifies users about specific use cases, via the `type` field.
// @Tags         Notifications
// @Accept       json
// @Produce      json
// @Param        Authorization  header  string                       true  "Insert your access token"  default(Bearer <Add access token here>)
// @Param        request        body    notifications.NotifyUserArg  true  "Request params"
// @Success      202            "ok - accepted"
// @Failure      400            {object}  server.ErrorResponse  "if validations fail"
// @Failure      401            {object}  server.ErrorResponse  "if not authorized"
// @Failure      403            {object}  server.ErrorResponse  "if not allowed"
// @Failure      404            {object}  server.ErrorResponse  "if user not found"
// @Failure      409            {object}  server.ErrorResponse  "if user already notified in the last X seconds"
// @Failure      422            {object}  server.ErrorResponse  "if syntax fails"
// @Failure      500            {object}  server.ErrorResponse
// @Failure      504            {object}  server.ErrorResponse  "if request times out"
// @Router       /notifications [POST].
func (s *service) NotifyUser(ctx context.Context, r server.ParsedRequest) server.Response {
	if err := s.notificationsProcessor.NotifyUser(ctx, &r.(*RequestNotifyUser).NotifyUserArg); err != nil {
		err = errors.Wrapf(err, "failed to notify user %#v", r.(*RequestNotifyUser).NotifyUserArg)
		if errors.Is(err, notifications.ErrPingingUserNotAllowed) {
			return *server.Forbidden(err)
		}
		if errors.Is(err, notifications.ErrNotFound) {
			return *server.NotFound(err, userNotFoundErrorCode)
		}
		if errors.Is(err, notifications.ErrDuplicate) {
			return *server.Conflict(err, userAlreadyNotifiedErrorCode)
		}

		return server.Unexpected(err)
	}

	return server.Response{Code: http.StatusAccepted}
}

func newRequestNotifyUser() server.ParsedRequest {
	return new(RequestNotifyUser)
}

func (req *RequestNotifyUser) SetAuthenticatedUser(user server.AuthenticatedUser) {
	if req.AuthenticatedUser.ID == "" {
		req.AuthenticatedUser.ID = user.ID
	}
}

func (req *RequestNotifyUser) GetAuthenticatedUser() server.AuthenticatedUser {
	return req.AuthenticatedUser
}

func (req *RequestNotifyUser) Validate() *server.Response {
	req.ActorUserID = req.AuthenticatedUser.ID
	if req.NotificationType == "" {
		req.NotificationType = notifications.PingNotification
	}
	req.NotificationType = strings.ToUpper(req.NotificationType)
	var valid bool
	for _, notificationType := range notifications.NotificationTypes {
		if req.NotificationType == notificationType {
			valid = true

			break
		}
	}
	if !valid {
		err := errors.Errorf("type:`%v` is not allowed. Allowed only any of: %#v ", req.NotificationType, notifications.NotificationTypes)

		return server.BadRequest(err, invalidPropertiesErrorCode)
	}

	return server.RequiredStrings(map[string]string{"userId": req.SubjectUserID})
}

func (req *RequestNotifyUser) Bindings(c *gin.Context) []func(obj interface{}) error {
	return []func(obj interface{}) error{c.ShouldBindJSON, server.ShouldBindAuthenticatedUser(c)}
}
