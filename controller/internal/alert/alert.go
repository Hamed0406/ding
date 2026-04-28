package alert

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/ding/ding/internal/diff"
)

type Config struct {
	TelegramToken  string
	TelegramChatID string
}

// Send dispatches change alerts via every configured channel.
// It is a no-op when changes is empty or no channels are configured.
func Send(cfg Config, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}
	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		return sendTelegram(cfg.TelegramToken, cfg.TelegramChatID, changes)
	}
	return nil
}

// SendTest sends a single test message to verify the Telegram credentials work.
func SendTest(cfg Config) error {
	if cfg.TelegramToken == "" || cfg.TelegramChatID == "" {
		return fmt.Errorf("telegram token and chat ID are required")
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramToken)
	resp, err := http.PostForm(apiURL, url.Values{
		"chat_id": {cfg.TelegramChatID},
		"text":    {"✅ Ding test message — your Telegram notifications are configured correctly."},
	})
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram: HTTP %d — check your token and chat ID", resp.StatusCode)
	}
	return nil
}

func sendTelegram(token, chatID string, changes []diff.Change) error {
	var sb strings.Builder
	sb.WriteString("Ding network changes:\n")
	for _, c := range changes {
		sb.WriteString("• ")
		sb.WriteString(c.String())
		sb.WriteByte('\n')
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	resp, err := http.PostForm(apiURL, url.Values{
		"chat_id": {chatID},
		"text":    {sb.String()},
	})
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	return nil
}
