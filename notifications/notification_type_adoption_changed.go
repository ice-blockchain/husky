// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"strconv"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

type (
	adoption struct {
		BaseMiningRate float64 `json:"baseMiningRate,omitempty" example:"1,243.02"`
		Milestone      uint64  `json:"milestone,omitempty" example:"1"`
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
	bpn := make([]*broadcastPushNotification[push.Notification[push.SubscriptionTopic]], 0, len(languages))
	bpnDelayed := make([]*broadcastPushNotification[push.DelayedNotification], 0, len(languages))
	data := struct{ BaseMiningRate float64 }{BaseMiningRate: adoption.BaseMiningRate}
	for language, tmpl := range languages {
		oldTopic := push.SubscriptionTopic(fmt.Sprintf("system_%v", language))
		newTopic := push.SubscriptionTopic(fmt.Sprintf("system_%v_v2", language))
		notif := &broadcastPushNotification[push.Notification[push.SubscriptionTopic]]{
			pn: &push.Notification[push.SubscriptionTopic]{
				Data:     map[string]string{"deeplink": fmt.Sprintf("%v://home?section=adoption", s.cfg.DeeplinkScheme)},
				Target:   oldTopic,
				Title:    tmpl.getTitle(nil),
				Body:     tmpl.getBody(data),
				ImageURL: s.pictureClient.DownloadURL("assets/push-notifications/adoption-change.png"),
			},
			sa: &sentAnnouncement{
				SentAt:   now,
				Language: language,
				sentAnnouncementPK: sentAnnouncementPK{
					Uniqueness:               strconv.FormatUint(adoption.Milestone, 10),
					NotificationType:         AdoptionChangedNotificationType,
					NotificationChannel:      PushNotificationChannel,
					NotificationChannelValue: fmt.Sprintf("system_%v", language),
				},
			},
		}

		bpn = append(bpn, notif)
		bpnDelayed = append(bpnDelayed, s.broadcastWithDelay(newTopic, notif))
	}

	return errors.Wrapf(multierror.Append(
		runConcurrently(ctx, s.broadcastPushNotification, bpn),
		runConcurrently(ctx, s.broadcastPushNotificationDelayed, bpnDelayed),
	).ErrorOrNil(),
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
				"baseMiningRate": adoption.BaseMiningRate,
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
				Value: strconv.FormatUint(adoption.Milestone, 10),
			},
		},
		sa: &sentAnnouncement{
			SentAt: now,
			sentAnnouncementPK: sentAnnouncementPK{
				Uniqueness:               strconv.FormatUint(adoption.Milestone, 10),
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
