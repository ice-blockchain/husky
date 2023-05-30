// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/coin"
	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

type (
	adoption struct {
		BaseMiningRate *coin.ICEFlake `json:"baseMiningRate,omitempty" example:"100000"`
		Milestone      uint64         `json:"milestone,omitempty" example:"1"`
	}
)

func (s *adoptionTableSource) Process(ctx context.Context, msg *messagebroker.Message) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	type (
		Adoption         = adoption
		adoptionSnapshot struct {
			*Adoption
			Before *Adoption `json:"before,omitempty"`
		}
	)
	message := new(adoptionSnapshot)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.Adoption == nil ||
		message.Adoption.Milestone == 0 ||
		message.Before == nil ||
		message.Before.Milestone == 0 ||
		message.Adoption.Milestone == message.Before.Milestone {
		return nil
	}

	return errors.Wrapf(multierror.Append(
		errors.Wrapf(s.broadcastPushNotifications(ctx, message.Adoption), "failed to broadcastPushNotifications for %#v", message.Adoption),
		errors.Wrapf(s.broadcastInAppNotifications(ctx, message.Adoption), "failed to broadcastInAppNotifications for %#v", message.Adoption),
		errors.Wrapf(s.broadcastEmailNotifications(ctx, message.Adoption), "failed to broadcastEmailNotifications for %#v", message.Adoption),
	).ErrorOrNil(), "atleast one type of adoption change broadcast failed for %#v", message)
}

func (s *adoptionTableSource) broadcastPushNotifications(ctx context.Context, adoption *adoption) error { //nolint:funlen // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	now := time.Now()
	languages := allPushNotificationTemplates[AdoptionChangedNotificationType]
	bpn := make([]*broadcastPushNotification, 0, len(languages))
	data := struct{ BaseMiningRate string }{BaseMiningRate: adoption.BaseMiningRate.UnsafeICE().String()}
	for language, tmpl := range languages {
		bpn = append(bpn, &broadcastPushNotification{
			pn: &push.Notification[push.SubscriptionTopic]{
				Data:     map[string]string{"deeplink": fmt.Sprintf("%v://home?section=adoption", s.cfg.DeeplinkScheme)},
				Target:   push.SubscriptionTopic(fmt.Sprintf("system_%v", language)),
				Title:    tmpl.getTitle(nil),
				Body:     tmpl.getBody(data),
				ImageURL: s.pictureClient.DownloadURL("assets/push-notifications/adoption-change.png"),
			},
			sa: &sentAnnouncement{
				SentAt:   now,
				Language: language,
				sentAnnouncementPK: sentAnnouncementPK{
					Uniqueness:               fmt.Sprint(adoption.Milestone),
					NotificationType:         AdoptionChangedNotificationType,
					NotificationChannel:      PushNotificationChannel,
					NotificationChannelValue: fmt.Sprintf("system_%v", language),
				},
			},
		})
	}

	return errors.Wrapf(runConcurrently(ctx, s.broadcastPushNotification, bpn),
		"failed to broadcast push atleast to languages for %v, args:%#v", AdoptionChangedNotificationType, bpn)
}

func (s *adoptionTableSource) broadcastInAppNotifications(ctx context.Context, adoption *adoption) error { //nolint:funlen // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	now := time.Now()
	bin := &broadcastInAppNotification{
		in: &inapp.Parcel{
			Time: now,
			Data: map[string]any{
				"baseMiningRate": adoption.BaseMiningRate.UnsafeICE().String(),
				"deeplink":       fmt.Sprintf("%v://home?section=adoption", s.cfg.DeeplinkScheme),
				"imageUrl":       s.pictureClient.DownloadURL("assets/push-notifications/adoption-change.png"),
			},
			Action: string(AdoptionChangedNotificationType),
			Actor: inapp.ID{
				Type:  "system",
				Value: "system",
			},
			Subject: inapp.ID{
				Type:  "adoptionMilestone",
				Value: fmt.Sprint(adoption.Milestone),
			},
		},
		sa: &sentAnnouncement{
			SentAt: now,
			sentAnnouncementPK: sentAnnouncementPK{
				Uniqueness:               fmt.Sprint(adoption.Milestone),
				NotificationType:         AdoptionChangedNotificationType,
				NotificationChannel:      InAppNotificationChannel,
				NotificationChannelValue: "system",
			},
		},
	}

	return errors.Wrapf(s.broadcastInAppNotification(ctx, bin), "failed to broadcastInAppNotification for %v,notif:%#v", AdoptionChangedNotificationType, bin)
}

func (s *adoptionTableSource) broadcastEmailNotifications(ctx context.Context, adoption *adoption) error {
	if s == nil || ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}

	return errors.Errorf("broadcasting adoption changes via email is not supported yet. adoption:%#v", adoption)
}
