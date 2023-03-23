// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	stdlibtime "time"

	"github.com/goccy/go-json"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/analytics"
	"github.com/ice-blockchain/wintr/analytics/tracking"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/time"
)

func (s *startedDaysOffSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	type dayOffStarted struct {
		StartedAt                   *time.Time `json:"startedAt,omitempty"`
		UserID                      string     `json:"userId,omitempty" `
		ID                          string     `json:"id,omitempty"`
		RemainingFreeMiningSessions uint64     `json:"remainingFreeMiningSessions,omitempty"`
	}
	message := new(dayOffStarted)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.UserID == "" {
		return nil
	}

	return errors.Wrap(executeConcurrently(func() error {
		return errors.Wrapf(s.sendAnalyticsSetUserAttributesCommandMessage(ctx, &analytics.SetUserAttributesCommand{
			Attributes: map[string]any{
				"Last mining start":  message.StartedAt.Format(stdlibtime.RFC3339),
				"Remaining days off": message.RemainingFreeMiningSessions,
			},
			UserID: message.UserID,
		}),
			"failed to sendAnalyticsSetUserAttributesCommandMessage %#v", message)
	}, func() error {
		return errors.Wrapf(s.sendAnalyticsTrackActionCommandMessage(ctx, &analytics.TrackActionCommand{
			Action: &tracking.Action{
				Name: "Day off",
			},
			ID:     fmt.Sprintf("day_off_%v", message.ID),
			UserID: message.UserID,
		}),
			"failed to sendAnalyticsTrackActionCommandMessage %#v", message)
	}, func() error {
		return errors.Wrapf(s.sendAnalyticsTrackActionCommandMessage(ctx, &analytics.TrackActionCommand{
			Action: &tracking.Action{
				Name: "Tap To Mine",
				Attributes: map[string]any{
					"Tap to Mine": "DayOff",
				},
			},
			ID:     fmt.Sprintf("tap_to_mine_%v", message.ID),
			UserID: message.UserID,
		}),
			"failed to sendAnalyticsTrackActionCommandMessage %#v", message)
	}), "at least one analytics command failed to execute")
}
