package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	botpkg "crypto-currency/internal/bot"
	ratespkg "crypto-currency/internal/rates"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	loadEnv()
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	botAPI, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatalf("failed to start telegram bot: %v", err)
	}
	botAPI.Debug = false

	ratesClient := ratespkg.NewRatesClient(&http.Client{Timeout: 15 * time.Second}, ratespkg.NewCache())
	handler := botpkg.NewHandler(botAPI, ratesClient)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updates := botAPI.GetUpdatesChan(updateConfig)

	for update := range updates {
		if update.Message != nil {
			handler.HandleMessage(update.Message)
			continue
		}
		if update.CallbackQuery != nil {
			handler.HandleCallback(update.CallbackQuery)
		}
	}
}

func loadEnv() {
	file, err := os.Open(".env")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		os.Setenv(key, value)
	}
}
