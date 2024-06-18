// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	"github.com/ice-blockchain/wintr/log"
)

type (
	telegramNotificationTemplate struct {
		body, buttonText, buttonLink *template.Template
		Body                         string `json:"body"`       //nolint:revive // That's intended.
		ButtonText                   string `json:"buttonText"` //nolint:revive // That's intended.
		ButtonLink                   string `json:"buttonLink"` //nolint:revive // That's intended.
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

func (t *telegramNotificationTemplate) getButtonLink(data any) string { //nolint:unparam // To be more generic.
	if data == nil {
		return t.ButtonLink
	}
	bf := new(bytes.Buffer)
	log.Panic(errors.Wrapf(t.buttonLink.Execute(bf, data), "failed to execute button url template for data:%#v", data))

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
			tmpl.ButtonLink = data.ButtonLink

			tmpl.buttonText = template.Must(template.New(fmt.Sprintf("telegram_%v_%v_button_text", notificationType, language)).Parse(data.ButtonText))
			tmpl.buttonLink = template.Must(template.New(fmt.Sprintf("telegram_%v_%v_button_link", notificationType, language)).Parse(data.ButtonLink))
			tmpl.body = template.Must(template.New(fmt.Sprintf("telegram_%v_%v_body", notificationType, language)).Parse(data.Body))
			allTelegramNotificationTemplates[notificationType][language] = &tmpl
		}
	}
}

func formatButtonLink(link, url string) string {
	if !strings.HasPrefix(link, "https") {
		link = fmt.Sprintf("%v%v", url, link)
	}

	return link
}
