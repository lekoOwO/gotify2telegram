package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func debug(s string, x ...interface{}) {
	log.Printf("Gotify2Telegram::"+s, x...)
}

func format_telegram_message(msg *GotifyMessage) string {
	// HTML Should be escaped here
	title := string(template.HTML("<b>" + template.HTMLEscapeString(msg.Title) + "</b>"))
	return fmt.Sprintf(
		"%s\n%s\n\nDate: %s",
		title,
		template.HTMLEscapeString(msg.Message),
		msg.Date,
	)
}

type Payload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ThreadId  string `json:"message_thread_id"`
	ParseMode string `json:"parse_mode"`
}

func send_msg_to_telegram(msg string, bot_token string, chat_id string, thread_id string) {
	step_size := 4090
	sending_message := ""
	for i := 0; i < len(msg); i += step_size {
		if i+step_size < len(msg) {
			sending_message = msg[i : i+step_size]
		} else {
			sending_message = msg[i:len(msg)]
		}

		data := Payload{
			ChatID:    chat_id,
			Text:      sending_message,
			ThreadId:  thread_id,
			ParseMode: "HTML",
		}
		payloadBytes, err := json.Marshal(data)
		if err != nil {
			log.Println("Create json false")
			return
		}
		body := bytes.NewReader(payloadBytes)

		req, err := http.NewRequest("POST", "https://api.telegram.org/bot"+bot_token+"/sendMessage", body)
		if err != nil {
			log.Println("Create request false")
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("Send request false: %v\n", err)
			return
		}
		defer resp.Body.Close()
	}
}

// DiscordPayload represents a Discord webhook payload that can include embeds.
type DiscordPayload struct {
	Username  string         `json:"username,omitempty"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Content   string         `json:"content,omitempty"`
	Embeds    []DiscordEmbed `json:"embeds,omitempty"`
}

type DiscordEmbed struct {
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	Color       int                  `json:"color,omitempty"`
	Timestamp   string               `json:"timestamp,omitempty"`
	Footer      *DiscordEmbedFooter  `json:"footer,omitempty"`
}

type DiscordEmbedFooter struct {
	Text string `json:"text,omitempty"`
}

// format_discord_embeds builds one or more embeds from GotifyMessage.
// It will split long descriptions into multiple embeds if necessary.
func format_discord_embeds(msg *GotifyMessage) []DiscordEmbed {
	title := template.HTMLEscapeString(msg.Title)
	body := template.HTMLEscapeString(msg.Message)

	// decide color by priority (example mapping)
	color := 0x2ECC71 // green default
	switch msg.Priority {
	case 5:
		color = 0xFF0000 // red
	case 4:
		color = 0xFFA500 // orange
	case 3:
		color = 0xFFFF00 // yellow
	case 2:
		color = 0x3498DB // blue
	}

	// Discord embed description limit is 4096 chars; split into chunks safely
	maxDesc := 3800
	runes := []rune(body)
	var embeds []DiscordEmbed
	for i := 0; i < len(runes); i += maxDesc {
		end := i + maxDesc
		if end > len(runes) {
			end = len(runes)
		}
		desc := string(runes[i:end])
		embed := DiscordEmbed{
			Title:       title,
			Description: desc,
			Color:       color,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Footer: &DiscordEmbedFooter{
				Text: fmt.Sprintf("Gotify Id: %d | Date: %s", msg.Id, msg.Date),
			},
		}
		// For subsequent chunks, omit the title to avoid repetition
		if i > 0 {
			embed.Title = ""
		}
		embeds = append(embeds, embed)
	}
	return embeds
}

// send_msg_to_discord posts embeds to a Discord webhook. It will send multiple requests if given multiple embeds.
func send_msg_to_discord(embeds []DiscordEmbed, webhookURL string, username string, avatarURL string) {
	if webhookURL == "" {
		return
	}

	// Discord allows multiple embeds in one payload; however to keep payload sizes safe
	// we will send up to 5 embeds per request (Discord limit is 10 embeds per request).
	maxEmbedsPerRequest := 5
	client := &http.Client{Timeout: 10 * time.Second}
	for start := 0; start < len(embeds); start += maxEmbedsPerRequest {
		end := start + maxEmbedsPerRequest
		if end > len(embeds) {
			end = len(embeds)
		}

		payload := DiscordPayload{
			Username:  username,
			AvatarURL: avatarURL,
			Embeds:    embeds[start:end],
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Println("Create discord json false")
			return
		}
		body := bytes.NewReader(payloadBytes)

		req, err := http.NewRequest("POST", webhookURL, body)
		if err != nil {
			log.Println("Create discord request false")
			return
		}
		req.Header.Set("Content-Type", "application/json")

		// Retry loop with exponential backoff and special handling for 429
		var resp *http.Response
		var attempt int
		maxRetries := 5
		for attempt = 0; attempt <= maxRetries; attempt++ {
			resp, err = client.Do(req)
			if err != nil {
				// network error, retry
				backoff := time.Duration(1<<attempt) * 200 * time.Millisecond
				time.Sleep(backoff)
				continue
			}

			// handle 429 (rate limit)
			if resp.StatusCode == 429 {
				ra := resp.Header.Get("Retry-After")
				resp.Body.Close()
				var wait time.Duration
				if ra != "" {
					// try seconds first
					if secs, parseErr := strconv.Atoi(strings.TrimSpace(ra)); parseErr == nil {
						wait = time.Duration(secs) * time.Second
					} else if t, parseErr2 := http.ParseTime(ra); parseErr2 == nil {
						wait = time.Until(t)
						if wait < 0 {
							wait = time.Second
						}
					} else {
						wait = time.Duration(1<<attempt) * 500 * time.Millisecond
					}
				} else {
					wait = time.Duration(1<<attempt) * 500 * time.Millisecond
				}
				time.Sleep(wait)
				continue
			}

			// retry on 5xx server errors
			if resp.StatusCode >= 500 && resp.StatusCode < 600 {
				resp.Body.Close()
				backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
				time.Sleep(backoff)
				continue
			}

			// other status codes (2xx or 4xx) - do not retry
			resp.Body.Close()
			break
		}

		if err != nil {
			fmt.Printf("Send discord request false: %v\n", err)
			return
		}
		// if we exhausted retries, log and continue to next batch
		if attempt > maxRetries {
			fmt.Printf("Send discord request failed after %d attempts\n", maxRetries)
		}
	}
}
