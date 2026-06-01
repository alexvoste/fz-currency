package bot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	localespkg "crypto-currency/internal/locales"
	ratespkg "crypto-currency/internal/rates"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	chart "github.com/wcharczuk/go-chart/v2"
)

type state string

const (
	stateDashboard state = "dashboard"
	stateAsset     state = "asset"
	stateSettings  state = "settings"
	stateLanguage  state = "language"
)

type session struct {
	Lang      string
	Asset     string
	Current   state
	Stack     []state
	MessageID int
}

type BotHandler struct {
	bot      *tgbotapi.BotAPI
	rates    *ratespkg.RatesClient
	sessions map[int64]*session
	mu       sync.RWMutex
}

func NewHandler(botAPI *tgbotapi.BotAPI, ratesClient *ratespkg.RatesClient) *BotHandler {
	return &BotHandler{
		bot:      botAPI,
		rates:    ratesClient,
		sessions: make(map[int64]*session),
	}
}

func (h *BotHandler) HandleMessage(message *tgbotapi.Message) {
	chatID := message.Chat.ID
	if message.IsCommand() && message.Command() == "start" {
		h.resetSession(chatID)
		h.renderDashboard(chatID, 0, false)
		return
	}

	reply := tgbotapi.NewMessage(chatID, localespkg.Translate(h.getLang(chatID), "useStart"))
	reply.ParseMode = tgbotapi.ModeHTML
	h.bot.Send(reply)
}

func (h *BotHandler) HandleCallback(callback *tgbotapi.CallbackQuery) {
	if callback == nil || callback.Message == nil {
		log.Printf("missing callback message")
		return
	}

	chatID := callback.Message.Chat.ID
	payload := callback.Data
	answer := tgbotapi.NewCallback(callback.ID, "")
	if _, err := h.bot.Request(answer); err != nil {
		log.Printf("callback answer failed: %v", err)
	}

	log.Printf("callback received chat=%d message=%d payload=%q", chatID, callback.Message.MessageID, payload)

	switch {
	case payload == "refresh":
		_ = h.deleteMessage(chatID, callback.Message.MessageID)
		h.renderDashboard(chatID, 0, false)
	case payload == "settings":
		h.pushState(chatID)
		h.setState(chatID, stateSettings)
		_ = h.deleteMessage(chatID, callback.Message.MessageID)
		h.renderSettings(chatID, 0, false)
	case payload == "language":
		h.pushState(chatID)
		h.setState(chatID, stateLanguage)
		_ = h.deleteMessage(chatID, callback.Message.MessageID)
		h.renderLanguage(chatID, 0, false)
	case strings.HasPrefix(payload, "loc:"):
		lang := strings.TrimPrefix(payload, "loc:")
		h.setLang(chatID, lang)
		_ = h.deleteMessage(chatID, callback.Message.MessageID)
		h.renderSettings(chatID, 0, false)
	case strings.HasPrefix(payload, "asset:"):
		asset := strings.TrimPrefix(payload, "asset:")
		h.pushState(chatID)
		h.setState(chatID, stateAsset)
		h.setAsset(chatID, asset)
		_ = h.deleteMessage(chatID, callback.Message.MessageID)
		h.renderAssetDetails(chatID, 0, asset, false)
	case strings.HasPrefix(payload, "trend:"):
		asset := strings.TrimPrefix(payload, "trend:")
		h.sendTrendChart(chatID, asset)
	case payload == "back":
		_ = h.deleteMessage(chatID, callback.Message.MessageID)
		h.handleBack(chatID, 0)
	default:
		log.Printf("unknown callback action: %s", payload)
	}
}

func (h *BotHandler) resetSession(chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions[chatID] = &session{Lang: localespkg.DefaultLanguage(), Current: stateDashboard, Stack: []state{stateDashboard}}
}

func (h *BotHandler) getSession(chatID int64) *session {
	h.mu.RLock()
	sess, ok := h.sessions[chatID]
	h.mu.RUnlock()
	if !ok || sess == nil {
		h.resetSession(chatID)
		h.mu.RLock()
		sess = h.sessions[chatID]
		h.mu.RUnlock()
	}
	return sess
}

func (h *BotHandler) setLang(chatID int64, lang string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sess := h.getSession(chatID)
	if !localespkg.IsSupported(lang) {
		lang = localespkg.DefaultLanguage()
	}
	sess.Lang = lang
}

func (h *BotHandler) getLang(chatID int64) string {
	sess := h.getSession(chatID)
	if sess.Lang == "" {
		return localespkg.DefaultLanguage()
	}
	return sess.Lang
}

func (h *BotHandler) setAsset(chatID int64, asset string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sess := h.getSession(chatID)
	sess.Asset = asset
}

func (h *BotHandler) getAsset(chatID int64) string {
	return h.getSession(chatID).Asset
}

func (h *BotHandler) setState(chatID int64, next state) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sess := h.getSession(chatID)
	sess.Current = next
}

func (h *BotHandler) pushState(chatID int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sess := h.getSession(chatID)
	if len(sess.Stack) == 0 || sess.Stack[len(sess.Stack)-1] != sess.Current {
		sess.Stack = append(sess.Stack, sess.Current)
	}
}

func (h *BotHandler) popState(chatID int64) state {
	h.mu.Lock()
	defer h.mu.Unlock()
	sess := h.getSession(chatID)
	if len(sess.Stack) <= 1 {
		sess.Stack = []state{stateDashboard}
		sess.Current = stateDashboard
		return stateDashboard
	}
	sess.Stack = sess.Stack[:len(sess.Stack)-1]
	sess.Current = sess.Stack[len(sess.Stack)-1]
	return sess.Current
}

func (h *BotHandler) renderDashboard(chatID int64, messageID int, edit bool) {
	lang := h.getLang(chatID)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	crypto, cryptoErr := h.rates.GetCryptoRates(ctx)
	fiat, fiatErr := h.rates.GetFiatRates(ctx)
	text := h.buildDashboardText(lang, crypto, fiat, cryptoErr, fiatErr)
	markup := h.dashboardKeyboard(lang)
	h.renderText(chatID, messageID, text, markup, edit)
}

func (h *BotHandler) renderSettings(chatID int64, messageID int, edit bool) {
	lang := h.getLang(chatID)
	text := fmt.Sprintf("<b>%s</b>\n\n%s", localespkg.Translate(lang, "settingsHeader"), localespkg.Translate(lang, "settingsInstruction"))
	markup := h.settingsKeyboard(lang)
	h.renderText(chatID, messageID, text, markup, edit)
}

func (h *BotHandler) renderLanguage(chatID int64, messageID int, edit bool) {
	lang := h.getLang(chatID)
	text := fmt.Sprintf("<b>%s</b>\n\n%s", localespkg.Translate(lang, "languageHeader"), localespkg.Translate(lang, "selectLanguage"))
	markup := h.languageKeyboard(lang)
	h.renderText(chatID, messageID, text, markup, edit)
}

func (h *BotHandler) renderAssetDetails(chatID int64, messageID int, asset string, edit bool) {
	lang := h.getLang(chatID)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	if isCryptoAsset(asset) {
		crypto, err := h.rates.GetCryptoRates(ctx)
		text := h.buildCryptoAssetText(lang, asset, crypto, err)
		markup := h.assetKeyboard(lang, asset)
		h.renderText(chatID, messageID, text, markup, edit)
		return
	}

	fiat, err := h.rates.GetFiatRates(ctx)
	text := h.buildFiatAssetText(lang, asset, fiat, err)
	markup := h.assetKeyboard(lang, asset)
	h.renderText(chatID, messageID, text, markup, edit)
}

func (h *BotHandler) handleBack(chatID int64, _ int) {
	current := h.popState(chatID)
	sess := h.getSession(chatID)
	_ = h.deleteMessage(chatID, sess.MessageID)
	switch current {
	case stateSettings:
		h.renderSettings(chatID, 0, false)
	case stateLanguage:
		h.renderLanguage(chatID, 0, false)
	case stateAsset:
		h.renderAssetDetails(chatID, 0, h.getAsset(chatID), false)
	default:
		h.renderDashboard(chatID, 0, false)
	}
}
func (h *BotHandler) renderText(chatID int64, messageID int, text string, markup tgbotapi.InlineKeyboardMarkup, edit bool) {
	if edit && messageID != 0 {
		ed := tgbotapi.EditMessageTextConfig{
			BaseEdit:              tgbotapi.BaseEdit{ChatID: chatID, MessageID: messageID, ReplyMarkup: &markup},
			Text:                  text,
			ParseMode:             tgbotapi.ModeHTML,
			DisableWebPagePreview: true,
		}
		if _, err := h.bot.Send(ed); err == nil {
			if sess := h.getSession(chatID); sess != nil {
				sess.MessageID = messageID
			}
			return
		} else {
			log.Printf("edit message failed: %v, deleting old message and sending new", err)
			_ = h.deleteMessage(chatID, messageID)
		}
	}

	h.sendTextMessage(chatID, text, markup)
}

func (h *BotHandler) sendTextMessage(chatID int64, text string, markup tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = true
	msg.ReplyMarkup = markup
	if sent, err := h.bot.Send(msg); err == nil {
		if sess := h.getSession(chatID); sess != nil {
			sess.MessageID = sent.MessageID
		}
	} else {
		log.Printf("send message failed: %v", err)
	}
}

func (h *BotHandler) deleteMessage(chatID int64, messageID int) error {
	del := tgbotapi.NewDeleteMessage(chatID, messageID)
	if _, err := h.bot.Request(del); err != nil {
		log.Printf("delete message failed: %v", err)
		return err
	}
	return nil
}

func (h *BotHandler) sendTrendChart(chatID int64, asset string) {
	lang := h.getLang(chatID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var points []float64
	var err error
	if isCryptoAsset(asset) {
		points, err = h.rates.GetCryptoHistory(ctx, assetCoingeckoID(asset), 7)
	} else {
		points, err = h.rates.GetFiatHistory(ctx, assetBase(asset), fiatTrendQuote(asset), 7)
	}
	if err != nil || len(points) == 0 {
		return
	}

	chartBytes, err := generateChart(points)
	if err != nil {
		return
	}

	caption := h.buildTrendCaption(lang, asset, points)
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{Name: "trend.png", Bytes: chartBytes})
	photo.Caption = caption
	photo.ParseMode = tgbotapi.ModeHTML
	photo.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "back"), "back"),
		),
	)
	h.bot.Send(photo)
}

func generateChart(points []float64) ([]byte, error) {
	xValues := make([]float64, len(points))
	for i := range points {
		xValues[i] = float64(i)
	}

	series := chart.ContinuousSeries{
		XValues: xValues,
		YValues: points,
		Style: chart.Style{
			StrokeColor: chart.ColorBlue,
			StrokeWidth: 2.5,
		},
	}

	graph := chart.Chart{
		Width:      820,
		Height:     360,
		Background: chart.Style{FillColor: chart.ColorBlack},
		Canvas:     chart.Style{FillColor: chart.ColorBlack},
		XAxis:      chart.XAxis{Style: chart.Style{Hidden: true}},
		YAxis:      chart.YAxis{Style: chart.Style{Hidden: true}},
		Series:     []chart.Series{series},
	}

	buffer := bytes.NewBuffer(nil)
	if err := graph.Render(chart.PNG, buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (h *BotHandler) buildDashboardText(lang string, crypto map[string]ratespkg.CryptoRate, fiat ratespkg.FiatRates, cryptoErr, fiatErr error) string {
	translator := localespkg.NewTranslator(lang)
	lines := []string{fmt.Sprintf("<b>%s</b>", translator.T("dashboardTitle")), "<i>──────────────</i>"}
	if cryptoErr == nil {
		for _, asset := range []string{"BTC", "ETH", "SOL", "TON"} {
			if item, ok := crypto[asset]; ok {
				lines = append(lines, fmt.Sprintf("<b>%s</b> %s <b>$%s</b> %s", asset, formatChange(item.Change24h), formatValue(item.Price), formatPercent(item.Change24h)))
			}
		}
	} else {
		lines = append(lines, translator.T("noData"))
	}
	if fiatErr == nil {
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s  <b>%s</b> %s", translator.T("usdEur"), formatValue(fiat.USDEUR), translator.T("usdRub"), formatValue(fiat.USDRUB)))
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("eurRub"), formatValue(fiat.EURRUB)))
	} else {
		lines = append(lines, translator.T("noData"))
	}
	lines = append(lines, "<i>──────────────</i>")
	lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("updated"), time.Now().UTC().Format("2006-01-02 15:04:05 UTC")))
	return strings.Join(lines, "\n")
}

func (h *BotHandler) buildCryptoAssetText(lang, asset string, crypto map[string]ratespkg.CryptoRate, err error) string {
	translator := localespkg.NewTranslator(lang)
	lines := []string{fmt.Sprintf("<b>%s %s</b>", asset, translator.T("assetHeader")), "<i>──────────────</i>"}
	if err != nil {
		lines = append(lines, translator.T("noData"))
		return strings.Join(lines, "\n")
	}
	if item, ok := crypto[asset]; ok {
		lines = append(lines, fmt.Sprintf("<b>%s</b> $%s", translator.T("price"), formatValue(item.Price)))
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("change24h"), formatPercent(item.Change24h)))
	} else {
		lines = append(lines, translator.T("noData"))
	}
	return strings.Join(lines, "\n")
}

func (h *BotHandler) buildFiatAssetText(lang, asset string, fiat ratespkg.FiatRates, err error) string {
	translator := localespkg.NewTranslator(lang)
	lines := []string{fmt.Sprintf("<b>%s %s</b>", asset, translator.T("assetHeader")), "<i>──────────────</i>"}
	if err != nil {
		lines = append(lines, translator.T("noData"))
		return strings.Join(lines, "\n")
	}
	switch asset {
	case "USD":
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("usdEur"), formatValue(fiat.USDEUR)))
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("usdRub"), formatValue(fiat.USDRUB)))
	case "EUR":
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("usdEur"), formatValue(1.0/fiat.USDEUR)))
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("eurRub"), formatValue(fiat.EURRUB)))
	case "RUB":
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("usdRub"), formatValue(1.0/fiat.USDRUB)))
		lines = append(lines, fmt.Sprintf("<b>%s</b> %s", translator.T("eurRub"), formatValue(1.0/fiat.EURRUB)))
	default:
		lines = append(lines, translator.T("noData"))
	}
	return strings.Join(lines, "\n")
}

func (h *BotHandler) buildTrendCaption(lang, asset string, points []float64) string {
	translator := localespkg.NewTranslator(lang)
	latest := points[len(points)-1]
	change := 0.0
	if len(points) > 1 && points[0] != 0 {
		change = ((latest - points[0]) / points[0]) * 100
	}
	return fmt.Sprintf("<b>%s</b>\n%s <b>$%s</b>\n%s <b>%s</b>", asset, translator.T("trend"), formatValue(latest), translator.T("change24h"), formatPercent(change))
}

func formatValue(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

func formatPercent(value float64) string {
	if value > 0 {
		return fmt.Sprintf("▲ %.2f%%", value)
	}
	if value < 0 {
		return fmt.Sprintf("▼ %.2f%%", value)
	}
	return fmt.Sprintf("• %.2f%%", value)
}

func formatChange(value float64) string {
	if value > 0 {
		return "▲"
	}
	if value < 0 {
		return "▼"
	}
	return "•"
}

func isCryptoAsset(asset string) bool {
	switch asset {
	case "BTC", "ETH", "SOL", "TON":
		return true
	}
	return false
}

func assetCoingeckoID(asset string) string {
	switch asset {
	case "BTC":
		return "bitcoin"
	case "ETH":
		return "ethereum"
	case "SOL":
		return "solana"
	case "TON":
		return "toncoin"
	}
	return ""
}

func assetBase(asset string) string {
	switch asset {
	case "USD":
		return "USD"
	case "EUR":
		return "EUR"
	case "RUB":
		return "RUB"
	}
	return "USD"
}

func fiatTrendQuote(asset string) string {
	switch asset {
	case "USD":
		return "EUR"
	case "EUR":
		return "USD"
	case "RUB":
		return "USD"
	}
	return "EUR"
}

func (h *BotHandler) dashboardKeyboard(lang string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("BTC", "asset:BTC"),
			tgbotapi.NewInlineKeyboardButtonData("ETH", "asset:ETH"),
			tgbotapi.NewInlineKeyboardButtonData("SOL", "asset:SOL"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("TON", "asset:TON"),
			tgbotapi.NewInlineKeyboardButtonData("USD", "asset:USD"),
			tgbotapi.NewInlineKeyboardButtonData("EUR", "asset:EUR"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("RUB", "asset:RUB"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "refresh"), "refresh"),
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "settings"), "settings"),
		),
	)
}

func (h *BotHandler) settingsKeyboard(lang string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "language"), "language"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "back"), "back"),
		),
	)
}

func (h *BotHandler) languageKeyboard(lang string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "languageEnglish"), "loc:en"),
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "languageRussian"), "loc:ru"),
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "languageChinese"), "loc:zh"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "back"), "back"),
		),
	)
}

func (h *BotHandler) assetKeyboard(lang, asset string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "trendButton"), "trend:"+asset),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(localespkg.Translate(lang, "back"), "back"),
		),
	)
}
