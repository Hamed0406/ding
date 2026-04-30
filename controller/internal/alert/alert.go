package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ding/ding/internal/diff"
)

type Config struct {
	TelegramToken  string
	TelegramChatID string
	WebhookURL     string
}

// Send dispatches change alerts via every configured channel.
// It is a no-op when changes is empty or no channels are configured.
func Send(cfg Config, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}
	var errs []string
	if cfg.TelegramToken != "" && cfg.TelegramChatID != "" {
		if err := sendTelegram(cfg.TelegramToken, cfg.TelegramChatID, changes); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if cfg.WebhookURL != "" {
		if err := sendWebhook(cfg.WebhookURL, changes); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
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

// SendTestWebhook fires a test payload to the given webhook URL.
func SendTestWebhook(webhookURL string) error {
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	payload := webhookPayload(
		[]diff.Change{{Kind: diff.KindNew, IP: "192.168.1.1", Desc: "test — your webhook is configured correctly"}},
		true,
	)
	return postWebhook(webhookURL, payload)
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

// sendWebhook posts a JSON change payload to a generic HTTP endpoint.
// The payload includes `text` (Slack), `content` (Discord), and `message`
// (ntfy.sh) keys — all carrying the same human-readable summary — so it
// works natively with all three without URL-sniffing.
func sendWebhook(webhookURL string, changes []diff.Change) error {
	return postWebhook(webhookURL, webhookPayload(changes, false))
}

type webhookChange struct {
	Kind string `json:"kind"`
	IP   string `json:"ip"`
	Desc string `json:"desc"`
}

func webhookPayload(changes []diff.Change, isTest bool) map[string]any {
	var sb strings.Builder
	if isTest {
		sb.WriteString("Ding test — your webhook is configured correctly.")
	} else {
		sb.WriteString("Ding network changes:\n")
		for _, c := range changes {
			sb.WriteString("• ")
			sb.WriteString(c.String())
			sb.WriteByte('\n')
		}
	}
	msg := strings.TrimSpace(sb.String())

	wcs := make([]webhookChange, len(changes))
	for i, c := range changes {
		wcs[i] = webhookChange{Kind: string(c.Kind), IP: c.IP, Desc: c.Desc}
	}

	event := "network_change"
	if isTest {
		event = "test"
	}

	return map[string]any{
		"event":     event,
		"text":      msg, // Slack incoming webhooks
		"content":   msg, // Discord incoming webhooks
		"message":   msg, // ntfy.sh
		"changes":   wcs,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

func postWebhook(webhookURL string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook: HTTP %d", resp.StatusCode)
	}
	return nil
}
