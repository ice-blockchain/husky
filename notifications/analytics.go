// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"

	"github.com/goccy/go-json"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/husky/analytics"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
)

func (r *repository) sendAnalyticsSetUserAttributesCommandMessage(ctx context.Context, cmd *analytics.SetUserAttributesCommand) error {
	valueBytes, err := json.MarshalContext(ctx, cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %#v", cmd)
	}
	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "husky"},
		Key:     cmd.UserID,
		Topic:   r.cfg.MessageBroker.ProducingTopics[0].Name,
		Value:   valueBytes,
	}
	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send `%v` message to broker", msg.Topic)
}

func (r *repository) sendAnalyticsTrackActionCommandMessage(ctx context.Context, cmd *analytics.TrackActionCommand) error {
	valueBytes, err := json.MarshalContext(ctx, cmd)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal %#v", cmd)
	}
	msg := &messagebroker.Message{
		Headers: map[string]string{"producer": "husky"},
		Key:     cmd.UserID,
		Topic:   r.cfg.MessageBroker.ProducingTopics[1].Name,
		Value:   valueBytes,
	}
	responder := make(chan error, 1)
	defer close(responder)
	r.mb.SendMessage(ctx, msg, responder)

	return errors.Wrapf(<-responder, "failed to send `%v` message to broker", msg.Topic)
}
