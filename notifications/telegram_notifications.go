// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/log"
)

type (
	telegramNotificationTemplate struct {
		body, buttonText *template.Template
		Body             string `json:"body"`       //nolint:revive // That's intended.
		ButtonText       string `json:"buttonText"` //nolint:revive // That's intended.
	}
)

func (t *telegramNotificationTemplate) getButtonText(data any) string {
	if data == nil {
		return t.ButtonText
	}
	bf := new(bytes.Buffer)
	log.Panic(errors.Wrapf(t.buttonText.Execute(bf, data), "failed to execute button text template for data:%#v", data))

	return bf.String()
}

func (t *telegramNotificationTemplate) getBody(data any) string {
	if data == nil {
		return t.Body
	}
	bf := new(bytes.Buffer)
	log.Panic(errors.Wrapf(t.body.Execute(bf, data), "failed to execute body template for data:%#v", data))

	return bf.String()
}

func loadTelegramNotificationTranslationTemplates() {
	const totalLanguages = 50
	allTelegramNotificationTemplates = make(map[NotificationType]map[languageCode]*telegramNotificationTemplate, len(AllTelegramNotificationTypes))
	for _, notificationType := range AllTelegramNotificationTypes {
		content, fErr := translations.ReadFile(fmt.Sprintf("translations/telegram/%v.txt", notificationType))
		if fErr != nil {
			panic(fErr)
		}
		allTelegramNotificationTemplates[notificationType] = make(map[languageCode]*telegramNotificationTemplate, totalLanguages)
		var translations map[string]*struct {
			Body       string `json:"body"`
			ButtonText string `json:"buttonText"`
			ButtonLink string `json:"buttonLink"`
		}
		err := json.Unmarshal(content, &translations)
		if err != nil {
			panic(err)
		}
		for language, data := range translations {
			var tmpl telegramNotificationTemplate
			tmpl.Body = data.Body
			tmpl.ButtonText = data.ButtonText

			tmpl.buttonText = template.Must(template.New(fmt.Sprintf("telegram_%v_%v_button_text", notificationType, language)).Parse(data.ButtonText))
			tmpl.body = template.Must(template.New(fmt.Sprintf("telegram_%v_%v_body", notificationType, language)).Parse(data.Body))
			allTelegramNotificationTemplates[notificationType][language] = &tmpl
		}
	}
}

//nolint:exhaustive // We know what cases need to be handled only.
func getTelegramDeeplink(nt NotificationType, cfg *config) string {
	urls := getSocialsMapURL(cfg)
	switch nt {
	case MiningExtendNotificationType, MiningEndingSoonNotificationType, MiningExpiredNotificationType, MiningNotActiveNotificationType:
		return fmt.Sprintf("%v/home", cfg.WebAppLink)
	case InviteFriendNotificationType:
		return fmt.Sprintf("%v/invite", cfg.WebAppLink)
	case SocialsFollowIceOnXNotificationType:
		return urls[string(SocialsFollowIceOnXNotificationType)]
	case SocialsFollowUsOnXNotificationType:
		return urls[string(SocialsFollowUsOnXNotificationType)]
	case SocialsFollowZeusOnXNotificationType:
		return urls[string(SocialsFollowZeusOnXNotificationType)]
	case SocialsFollowIONOnTelegramNotificationType:
		return urls[string(SocialsFollowIONOnTelegramNotificationType)]
	case SocialsFollowOurTelegramNotificationType:
		return urls[string(SocialsFollowOurTelegramNotificationType)]
	case CoinBadgeUnlockedNotificationType, LevelBadgeUnlockedNotificationType, SocialBadgeUnlockedNotificationType:
		return fmt.Sprintf("%v/profile/badges", cfg.WebAppLink)
	case LevelChangedNotificationType:
		return fmt.Sprintf("%v/profile", cfg.WebAppLink)
	default:
		log.Panic(fmt.Sprintf("wrong notification type:%v", nt))
	}

	return ""
}

func getSocialsMapURL(cfg *config) map[string]string {
	if len(cfg.Socials) == 0 {
		log.Panic("no urls for socials")
	}
	urls := make(map[string]string, len(cfg.Socials))
	for ix := range cfg.Socials {
		urls[cfg.Socials[ix].NotificationType] = cfg.Socials[ix].Link
	}

	return urls
}
