// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"
	stdlibtime "time"

	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"

	"github.com/ice-blockchain/eskimo/users"
	storage "github.com/ice-blockchain/wintr/connectors/storage/v2"
	"github.com/ice-blockchain/wintr/log"
	"github.com/ice-blockchain/wintr/notifications/telegram"
	"github.com/ice-blockchain/wintr/time"
)

type (
	telegramNotificationTemplate struct {
		body, inviteText, altBody *template.Template
		Body                      string               `json:"body"`       //nolint:revive // That's intended.
		AltBody                   string               `json:"altBody"`    //nolint:revive // That's intended.
		InviteText                string               `json:"inviteText"` //nolint:revive // That's intended.
		ButtonText                []string             `json:"buttonText"`
		buttonText                []*template.Template //nolint:revive // That's intended.
	}
)

func (t *telegramNotificationTemplate) getButtonText(data any, ix int) string { //nolint:unparam // .
	if ix > len(t.buttonText)-1 {
		return ""
	}
	if data == nil {
		return t.ButtonText[ix]
	}
	bf := new(bytes.Buffer)
	log.Panic(errors.Wrapf(t.buttonText[ix].Execute(bf, data), "failed to execute button text 1 template for data:%#v", data))

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

func (t *telegramNotificationTemplate) getAltBody(data any) string {
	if data == nil {
		return t.AltBody
	}
	bf := new(bytes.Buffer)
	log.Panic(errors.Wrapf(t.altBody.Execute(bf, data), "failed to execute altBody template for data:%#v", data))

	return bf.String()
}

func (t *telegramNotificationTemplate) getInviteText(data any) string {
	if t.inviteText == nil {
		return ""
	}
	if data == nil {
		return t.InviteText
	}
	bf := new(bytes.Buffer)
	if err := t.inviteText.Execute(bf, data); err != nil {
		return ""
	}

	return bf.String()
}

func loadTelegramNotificationTranslationTemplates() { //nolint:funlen,revive,gocognit // .
	const totalLanguages = 50
	allTelegramNotificationTemplates = make(map[NotificationType]map[languageCode]*telegramNotificationTemplate, len(AllTelegramNotificationTypes))
	for _, notificationType := range AllTelegramNotificationTypes {
		content, fErr := translations.ReadFile(fmt.Sprintf("translations/telegram/%v.txt", notificationType))
		if fErr != nil {
			panic(fErr)
		}
		allTelegramNotificationTemplates[notificationType] = make(map[languageCode]*telegramNotificationTemplate, totalLanguages)
		var translations map[string]*struct {
			Body       string   `json:"body"`
			AltBody    string   `json:"altBody"`
			InviteText string   `json:"inviteText"`
			ButtonText []string `json:"buttonText"`
		}
		err := json.Unmarshal(content, &translations)
		if err != nil {
			panic(err)
		}
		for language, data := range translations {
			var tmpl telegramNotificationTemplate
			tmpl.Body = data.Body
			tmpl.AltBody = data.AltBody
			if notificationType == InviteFriendNotificationType {
				tmpl.InviteText = data.InviteText
				tmpl.inviteText = template.Must(template.New(fmt.Sprintf("telegram_%v_%v_invite_text", notificationType, language)).Parse(data.InviteText))
			}
			if tmpl.AltBody != "" {
				tmpl.altBody = template.Must(template.New(fmt.Sprintf("push_%v_%v_alt_body", notificationType, language)).Parse(data.AltBody))
			}
			tmpl.ButtonText = data.ButtonText
			for ix := range data.ButtonText {
				tmpl.buttonText = append(tmpl.buttonText, template.Must(template.New(fmt.Sprintf("telegram_%v_%v_button_text_%v", notificationType, language, ix)).Parse(data.ButtonText[ix]))) //nolint:lll // .
			}
			tmpl.body = template.Must(template.New(fmt.Sprintf("telegram_%v_%v_body", notificationType, language)).Parse(data.Body))
			allTelegramNotificationTemplates[notificationType][language] = &tmpl
		}
	}
}

//nolint:exhaustive // We know what cases need to be handled only.
func getTelegramDeeplink(nt NotificationType, cfg *config, username, inviteText string) string {
	switch nt {
	case MiningExtendNotificationType, MiningEndingSoonNotificationType, MiningExpiredNotificationType, MiningNotActiveNotificationType:
		return cfg.WebAppLink
	case InviteFriendNotificationType:
		return fmt.Sprintf("%[1]v?url=%[2]v/@%[3]v&text=%[4]v", cfg.InviteURL, url.QueryEscape(cfg.WebSiteURL), url.QueryEscape(username), url.QueryEscape(inviteText)) //nolint:lll // .
	case CoinBadgeUnlockedNotificationType, LevelBadgeUnlockedNotificationType, SocialBadgeUnlockedNotificationType:
		return fmt.Sprintf("%v?startapp=goto_profile_badges", cfg.WebAppLink)
	case LevelChangedNotificationType:
		return fmt.Sprintf("%v?startapp=goto_profile", cfg.WebAppLink)
	case ReplyNotificationType:
		return cfg.WebAppLink
	case NewReferralNotificationType:
		return fmt.Sprintf("%v?startapp=goto_team", cfg.WebAppLink)
	default:
		log.Panic(fmt.Sprintf("wrong notification type:%v", nt))
	}

	return ""
}

func prepareTelegramButtonsForSocialNotificationType(cfg *config, buttonTexts []string) []telegram.Button {
	urls := getSocialsMapURL(cfg)
	if len(urls) != len(buttonTexts) {
		log.Error(errors.New("socials cfg/translation misconfiguration"))

		return nil
	}
	res := make([]telegram.Button, 0, len(buttonTexts))
	for ix, text := range buttonTexts {
		res = append(res, telegram.Button{
			Text: text,
			URL:  urls[ix],
		})
	}

	return res
}

func getSocialsMapURL(cfg *config) map[int]string {
	if len(cfg.Socials) == 0 {
		log.Panic("no urls for socials")
	}
	urls := make(map[int]string, len(cfg.Socials))
	for ix := range cfg.Socials {
		urls[ix] = cfg.Socials[ix].Link
	}

	return urls
}

func (r *repository) startGetUpdatesTelegramLongPolling(ctx context.Context) {
	ticker := stdlibtime.NewTicker(1 * stdlibtime.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			wg := new(sync.WaitGroup)
			wg.Add(len(r.cfg.TelegramBots))
			for botName, bot := range r.cfg.TelegramBots {
				go func(botName, botToken string) {
					defer wg.Done()
					reqCtx, cancel := context.WithTimeout(ctx, 1*stdlibtime.Minute)
					if err := r.getTelegramLongPollingUpdates(reqCtx, botName, botToken); err != nil {
						log.Error(errors.Wrapf(err, "failed to get updates long polling for botName:%v", botName))
					}
					cancel()
				}(botName, bot.BotToken)
			}
			wg.Wait()
		case <-ctx.Done():
			return
		}
	}
}

func (r *repository) getTelegramLongPollingUpdates(ctx context.Context, botName, botToken string) error {
	nextOffset := int64(0)
	for {
		upd, gErr := r.telegramNotificationsClient.GetUpdates(ctx, &telegram.GetUpdatesArg{
			BotToken:       botToken,
			AllowedUpdates: []string{"message", "callback_query"},
			Limit:          telegramLongPollingLimit,
			Offset:         nextOffset,
		})
		if gErr != nil {
			return errors.Wrapf(gErr, "can't get updates for bot:%v offset: %v", botName, nextOffset)
		}
		if len(upd) == 0 {
			break
		}
		offset, notifications, err := r.handleTelegramUpdates(ctx, upd, botToken, botName)
		if err != nil {
			return errors.Wrapf(err, "can't handle telegram updates for bot:%v for:%v", botName, upd)
		}
		nextOffset = offset

		if rErr := runConcurrentlyBatch(ctx, r.sendTelegramNotification, notifications, func(arg *telegramNotification, err error) {
			log.Error(errors.Wrapf(err, "can't send telegram command notification for bot:%v for:%+v", botName, arg))
		}, func(_ *telegramNotification) {}); rErr != nil {
			return errors.Wrapf(rErr, "can't send telegram command notification batch for bot:%v, for:%+v", botName, notifications)
		}
	}

	return nil
}

//nolint:funlen,gocognit,revive // .
func (r *repository) handleTelegramUpdates(
	ctx context.Context, updates []*telegram.Update, botToken, botName string,
) (nextOffset int64, notif []*telegramNotification, err error) {
	var (
		maxUpdateID    = int64(0)
		now            = time.Now()
		uniquenessTime = fmt.Sprintf("%v:%02d:%02d %02d:%02d:%02d", now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
		notifications  = make([]*telegramNotification, 0, len(updates))
	)
	userInfoMap, err := r.getTelegramReplyUserInfo(ctx, updates)
	if err != nil {
		return 0, nil, errors.Wrapf(err, "can't get user ids by telegram user ids for bot:%v for:%#v", botName, updates)
	}
	if len(userInfoMap) == 0 {
		return 0, nil, nil
	}
	for _, upd := range updates {
		if upd.UpdateID > maxUpdateID {
			maxUpdateID = upd.UpdateID
		}
		var (
			skip                          bool
			id, username, text, messageID string
		)
		if upd.Message != nil {
			skip, id, username, text, messageID = extractDataFromMessage(upd)
		} else if upd.CallbackQuery != nil {
			skip, id, username, text, messageID = extractDataFromCallbackQuery(upd)
		}
		if skip {
			continue
		}
		if _, ok := userInfoMap[id]; !ok {
			log.Error(errors.New("no such telegram user id"), id)

			continue
		}
		notificationType := notificationTypeByCommand(TelegramCommand(text))
		if notificationType == "" {
			log.Warn("wrong command provided", notificationType)

			continue
		}
		tmpl, found := allTelegramNotificationTemplates[notificationType][userInfoMap[id].Language]
		if !found {
			log.Warn(fmt.Sprintf("language `%v` was not found in the `%v` telegram notifications config", userInfoMap[id].Language, notificationType))
			tmpl, found = allTelegramNotificationTemplates[notificationType][defaultLanguage]
			if !found {
				log.Panic(fmt.Sprintf("no default translations provided for notification, lang:%v, notificationType:%v", userInfoMap[id].Language, notificationType)) //nolint:lll // .
			}
		}
		notification := &telegramNotification{
			tn: &telegram.Notification{
				ChatID: userInfoMap[id].TelegramUserID,
				Text: tmpl.getBody(users.JSON{
					"TenantName": r.cfg.TenantName,
					"TokenName":  r.cfg.TokenName,
					"Username":   username,
				}),
				BotToken: botToken,
			},
			sn: &sentNotification{
				SentAt:   now,
				Language: userInfoMap[id].Language,
				sentNotificationPK: sentNotificationPK{
					UserID:                   userInfoMap[id].UserID,
					Uniqueness:               fmt.Sprintf("command_%v_%v_%v", notificationType, messageID, uniquenessTime),
					NotificationType:         notificationType,
					NotificationChannel:      TelegramNotificationChannel,
					NotificationChannelValue: messageID,
				},
			},
		}
		switch notificationType { //nolint:exhaustive // We know for sure the commands to here.
		case StartCommandNotificationType:
			notification.tn.Buttons = append(notification.tn.Buttons, prepareTelegramButtonsForStartCommandNotificationType(r.cfg, tmpl.ButtonText)...)
		case SocialsNotificationType:
			notification.tn.Buttons = append(notification.tn.Buttons, prepareTelegramButtonsForSocialNotificationType(r.cfg, tmpl.ButtonText)...)
		default:
			continue
		}
		notifications = append(notifications, notification)
	}

	return maxUpdateID + 1, notifications, nil
}

//nolint:gocritic,revive // .
func extractDataFromMessage(upd *telegram.Update) (skip bool, id, username, text, messageID string) {
	if upd.Message == nil {
		return false, "", "", "", ""
	}
	if upd.Message.From.IsBot || !isCommand(upd) || !isAllowedCommand(TelegramCommand(upd.Message.Text)) {
		log.Warn("The message is sent through the bot or is not a command", upd)

		return false, "", "", "", ""
	}
	id = strconv.FormatInt(upd.Message.From.ID, 10)
	if upd.Message.From.Username == "" {
		log.Error(errors.Wrapf(errors.New("no username"), "no username for telegram user id:%v", upd.Message.From.ID))

		return true, "", "", "", ""
	}
	username = upd.Message.From.Username
	text = upd.Message.Text
	messageID = strconv.FormatInt(upd.Message.MessageID, 10)

	return skip, id, username, text, messageID
}

//nolint:gocritic,revive // .
func extractDataFromCallbackQuery(upd *telegram.Update) (skip bool, id, username, text, messageID string) {
	if upd.CallbackQuery == nil {
		return false, "", "", "", ""
	}
	if upd.CallbackQuery.From.IsBot || !isAllowedCommand(TelegramCommand(upd.CallbackQuery.Data)) {
		log.Warn("the message is sent through the bot or is not a command", upd)

		return true, "", "", "", ""
	}
	id = strconv.FormatInt(upd.CallbackQuery.From.ID, 10)
	if upd.CallbackQuery.From.Username == "" {
		log.Error(errors.Wrapf(errors.New("no username"), "no username for telegram user id:%v", upd.CallbackQuery.From.ID))

		return true, "", "", "", ""
	}
	username = upd.CallbackQuery.From.Username
	text = upd.CallbackQuery.Data
	messageID = upd.CallbackQuery.ID

	return skip, id, username, text, messageID
}

func prepareTelegramButtonsForStartCommandNotificationType(cfg *config, buttonTexts []string) []telegram.Button {
	res := make([]telegram.Button, 0, len(buttonTexts))
	res = append(res, telegram.Button{
		Text: buttonTexts[0],
		URL:  cfg.WebAppLink,
	}, telegram.Button{
		Text:         buttonTexts[1],
		CallbackData: string(SocialsTelegramCommand),
	}, telegram.Button{
		Text: buttonTexts[2],
		URL:  fmt.Sprintf("%v/%v", cfg.WebSiteURL, "knowledge-base"),
	})

	return res
}

func notificationTypeByCommand(command TelegramCommand) NotificationType {
	switch command {
	case StartTelegramCommand:
		return StartCommandNotificationType
	case SocialsTelegramCommand:
		return SocialsNotificationType
	default:
		return ""
	}
}

func isCommand(upd *telegram.Update) bool {
	isCommand := false
	for _, entity := range upd.Message.Entities {
		if entity.Type == telegram.BotCommandType {
			isCommand = true
		}
	}

	return isCommand
}

func isAllowedCommand(command TelegramCommand) bool {
	available := false
	for _, cmd := range availableBotCommands {
		if strings.Contains(string(command), string(cmd)) {
			available = true

			break
		}
	}

	return available
}

func (r *repository) getTelegramReplyUserInfo(ctx context.Context, updates []*telegram.Update) (res map[telegramUserID]*telegramUserInfo, err error) {
	if len(updates) == 0 {
		return nil, err
	}
	telegramUserIDs := make([]telegramUserID, 0, len(updates))
	for _, upd := range updates {
		if upd.Message != nil {
			telegramUserIDs = append(telegramUserIDs, strconv.FormatInt(upd.Message.From.ID, 10))
		} else if upd.CallbackQuery != nil {
			telegramUserIDs = append(telegramUserIDs, strconv.FormatInt(upd.CallbackQuery.From.ID, 10))
		}
	}
	sql := `SELECT 
				user_id,
				COALESCE(telegram_user_id, '') AS telegram_user_id,
				language
			FROM users 
			WHERE telegram_user_id = ANY($1)`
	result, err := storage.Select[telegramUserInfo](ctx, r.db, sql, telegramUserIDs)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get user ids for telegram user ids:%#v", telegramUserIDs)
	}
	if result == nil {
		return nil, err
	}
	res = make(map[telegramUserID]*telegramUserInfo, len(result))
	for _, val := range result {
		res[val.TelegramUserID] = val
	}

	return res, nil
}

func (r *repository) sendTelegramNotification(ctx context.Context, tn *telegramNotification) error {
	if ctx.Err() != nil {
		return errors.Wrap(ctx.Err(), "unexpected deadline")
	}
	if err := r.insertSentNotification(ctx, tn.sn); err != nil {
		return errors.Wrapf(err, "failed to insert %#v", tn.sn)
	}
	responder := make(chan error, 1)
	defer close(responder)
	if err := r.telegramNotificationsClient.Send(ctx, tn.tn); err != nil {
		return multierror.Append( //nolint:wrapcheck // Not needed.
			errors.Wrapf(err, "failed to send telegram notification:%#v, desired to be sent:%#v", tn.sn, tn.sn),
			errors.Wrapf(r.deleteSentNotification(ctx, tn.sn), "failed to delete SENT_NOTIFICATIONS as a rollback for %#v", tn.sn),
		).ErrorOrNil()
	}

	return nil
}
