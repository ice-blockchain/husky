// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"context"
	"fmt"
	"net/url"

	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	messagebroker "github.com/ice-blockchain/wintr/connectors/message_broker"
	"github.com/ice-blockchain/wintr/notifications/inapp"
	"github.com/ice-blockchain/wintr/notifications/push"
	"github.com/ice-blockchain/wintr/time"
)

type (
	news struct {
		*NotificationChannels
		ID       string `json:"id,omitempty" example:"did:ethr:0x4B73C58370AEfcEf86A6021afCDe5673511376B2"`
		Language string `json:"language,omitempty" example:"en"`
		ImageURL string `json:"imageUrl,omitempty" example:"https://somewebsite.com/blockchain.jpg"`
		URL      string `json:"url,omitempty" example:"https://somewebsite.com/blockchain"`
	}
)

func (s *newsTableSource) Process(ctx context.Context, msg *messagebroker.Message) error { //nolint:funlen,gocyclo,revive,cyclop // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline while processing message")
	}
	if len(msg.Value) == 0 {
		return nil
	}
	message := new(news)
	if err := json.UnmarshalContext(ctx, msg.Value, message); err != nil {
		return errors.Wrapf(err, "cannot unmarshal %v into %#v", string(msg.Value), message)
	}
	if message.ID == "" ||
		message.NotificationChannels == nil ||
		message.NotificationChannels.NotificationChannels == nil ||
		len(*message.NotificationChannels.NotificationChannels) == 0 {
		return nil
	}
	notificationChannels := make(map[NotificationChannel]bool, 1+1+1)
	for _, channel := range *message.NotificationChannels.NotificationChannels {
		notificationChannels[channel] = true
	}
	errs := make([]error, 0, 1+1+1)
	if notificationChannels[PushNotificationChannel] || notificationChannels[PushOrFallbackToEmailNotificationChannel] {
		errs = append(errs, errors.Wrapf(s.broadcastPushNotifications(ctx, message), "failed to broadcastPushNotifications for news:%#v", message))
	}
	if notificationChannels[InAppNotificationChannel] || notificationChannels[PushNotificationChannel] || notificationChannels[PushOrFallbackToEmailNotificationChannel] { //nolint:lll // .
		errs = append(errs, errors.Wrapf(s.broadcastInAppNotifications(ctx, message), "failed to broadcastInAppNotifications for news:%#v", message))
	}
	if notificationChannels[EmailNotificationChannel] || notificationChannels[PushOrFallbackToEmailNotificationChannel] {
		errs = append(errs, errors.Wrapf(s.broadcastEmailNotifications(ctx, notificationChannels[PushOrFallbackToEmailNotificationChannel], message), "failed to broadcastEmailNotifications for news:%#v", message)) //nolint:lll // .
	}

	return errors.Wrapf(multierror.Append(nil, errs...).ErrorOrNil(), "atleast one type of news broadcast failed for %#v", message)
}

func (s *newsTableSource) broadcastPushNotifications(ctx context.Context, newsArticle *news) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	tmpl, found := allPushNotificationTemplates[NewsAddedNotificationType][newsArticle.Language]
	if !found {
		return errors.Errorf("language `%v` was not found in the `%v` push config", newsArticle.Language, NewsAddedNotificationType)
	}
	now := time.Now()
	target := fmt.Sprintf("news_%v", newsArticle.Language)
	bpn := &broadcastPushNotification{
		pn: &push.Notification[push.SubscriptionTopic]{
			Data:     s.pushNotificationData(newsArticle),
			Target:   push.SubscriptionTopic(target),
			Title:    tmpl.getTitle(nil),
			Body:     tmpl.getBody(nil),
			ImageURL: newsArticle.ImageURL,
		},
		sa: &sentAnnouncement{
			SentAt:   now,
			Language: newsArticle.Language,
			sentAnnouncementPK: sentAnnouncementPK{
				Uniqueness:               newsArticle.ID,
				NotificationType:         NewsAddedNotificationType,
				NotificationChannel:      PushNotificationChannel,
				NotificationChannelValue: target,
			},
		},
	}

	return errors.Wrapf(s.broadcastPushNotification(ctx, bpn), "failed to broadcastPushNotification(%v) %#v", NewsAddedNotificationType, bpn)
}

func (s *newsTableSource) broadcastInAppNotifications(ctx context.Context, newsArticle *news) error { //nolint:funlen // .
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	now := time.Now()
	bin := &broadcastInAppNotification{
		in: &inapp.Parcel{
			Time:   now,
			Data:   s.inAppNotificationData(newsArticle),
			Action: string(NewsAddedNotificationType),
			Actor: inapp.ID{
				Type:  "system",
				Value: "system",
			},
			Subject: inapp.ID{
				Type:  "(language,newsId)",
				Value: fmt.Sprintf("(%v,%v)", newsArticle.Language, newsArticle.ID),
			},
		},
		sa: &sentAnnouncement{
			SentAt:   now,
			Language: newsArticle.Language,
			sentAnnouncementPK: sentAnnouncementPK{
				Uniqueness:               newsArticle.ID,
				NotificationType:         NewsAddedNotificationType,
				NotificationChannel:      InAppNotificationChannel,
				NotificationChannelValue: "system",
			},
		},
	}

	return errors.Wrapf(s.broadcastInAppNotification(ctx, bin), "failed to broadcastInAppNotification(%v) %#v", NewsAddedNotificationType, bin)
}

func (s *newsTableSource) broadcastEmailNotifications(ctx context.Context, fallbackOnly bool, newsArticle *news) error {
	if s == nil || ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}

	return errors.Errorf("broadcasting news via email is not supported yet. isFallback:%v, newsArticle:%#v", fallbackOnly, newsArticle)
}

func (s *newsTableSource) pushNotificationData(newsArticle *news) map[string]string {
	return map[string]string{
		"deeplink": s.deeplink(newsArticle),
	}
}

func (s *newsTableSource) inAppNotificationData(newsArticle *news) map[string]any {
	return map[string]any{
		"deeplink": s.deeplink(newsArticle),
		"imageUrl": newsArticle.ImageURL,
	}
}

func (s *newsTableSource) deeplink(newsArticle *news) string {
	return fmt.Sprintf("%v://browser?contentType=news&contentId=%v&contentLanguage=%v&url=%v",
		s.cfg.DeeplinkScheme, newsArticle.ID, newsArticle.Language, url.QueryEscape(newsArticle.URL))
}
