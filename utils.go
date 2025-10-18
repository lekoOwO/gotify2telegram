package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"io"
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
// helper: choose color based on priority
func discordColorForPriority(p uint32) int {
	switch p {
	case 5:
		return 0xFF0000
	case 4:
		return 0xFFA500
	case 3:
		return 0xFFFF00
	case 2:
		return 0x3498DB
	default:
		return 0x2ECC71
	}
}

// helper: compute allowed description rune count per embed
func allowedDescFor(title, footer string) int {
	const maxPerEmbed = 6000
	const overheadMargin = 200
	titleLen := len([]rune(title))
	footerLen := len([]rune(footer))
	allowed := maxPerEmbed - titleLen - footerLen - overheadMargin
	if allowed < 200 {
		allowed = 200
	}
	return allowed
}

// parse message into segments of code blocks and plain text
type segment struct{ isCode bool; lang, text string }

func parseSegments(body string) []segment {
	codeRe := regexp.MustCompile("(?s)```.*?```")
	idxs := codeRe.FindAllStringIndex(body, -1)
	segs := []segment{}
	last := 0
	for _, id := range idxs {
		if id[0] > last {
			segs = append(segs, segment{isCode: false, text: body[last:id[0]]})
		}
		block := body[id[0]:id[1]]
		inner := strings.TrimPrefix(strings.TrimSuffix(block, "```"), "```")
		lang := ""
		code := inner
		if n := strings.Index(inner, "\n"); n >= 0 {
			lang = strings.TrimSpace(inner[:n])
			code = inner[n+1:]
		}
		segs = append(segs, segment{isCode: true, lang: lang, text: code})
		last = id[1]
	}
	if last < len(body) {
		segs = append(segs, segment{isCode: false, text: body[last:]})
	}
	return segs
}

// build embeds from segments while preserving code blocks
func buildEmbedsFromSegments(title string, segs []segment, footerText string, priority uint32) []DiscordEmbed {
	color := discordColorForPriority(priority)
	allowed := allowedDescFor(title, footerText)
	var embeds []DiscordEmbed
	cur := []rune{}
	hasTitle := true
	flush := func() {
		t := ""
		if hasTitle {
			t = title
		}
		embeds = append(embeds, DiscordEmbed{Title: t, Description: string(cur), Color: color, Timestamp: "", Footer: &DiscordEmbedFooter{Text: footerText}})
		hasTitle = false
		cur = []rune{}
		allowed = allowedDescFor("", footerText)
	}

	for _, s := range segs {
		if s.isCode {
			opener := "```"
			if s.lang != "" { opener += s.lang + "\n" } else { opener += "\n" }
			closer := "```"
			r := []rune(s.text)
			i := 0
			for i < len(r) {
				remaining := allowed - len(cur) - len([]rune(opener)) - len([]rune(closer))
				if remaining <= 0 { flush(); continue }
				take := remaining
				if take > len(r)-i { take = len(r)-i }
				frag := string(r[i : i+take])
				cur = append(cur, []rune(opener+frag+"\n"+closer)...)
				i += take
				if i < len(r) { flush() }
			}
		} else {
			r := []rune(s.text)
			i := 0
			for i < len(r) {
				remaining := allowed - len(cur)
				if remaining <= 0 { flush(); continue }
				take := remaining
				if take > len(r)-i { take = len(r)-i }
				cur = append(cur, r[i:i+take]...)
				i += take
				if i < len(r) { flush() }
			}
		}
	}
	if len(cur) > 0 || len(embeds) == 0 { flush() }
	return embeds
}

func format_discord_embeds(msg *GotifyMessage) []DiscordEmbed {
	title := msg.Title
	footerText := fmt.Sprintf("Gotify Id: %d", msg.Id)
	segs := parseSegments(msg.Message)
	embeds := buildEmbedsFromSegments(title, segs, footerText, msg.Priority)
	// set timestamps to message date
	for i := range embeds {
		embeds[i].Timestamp = msg.Date
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

		// Retry loop with exponential backoff and special handling for 429
		var resp *http.Response
		var attempt int
		maxRetries := 5
		for attempt = 0; attempt <= maxRetries; attempt++ {
			req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(payloadBytes))
			if err != nil {
				log.Println("Create discord request false")
				return
			}
			req.Header.Set("Content-Type", "application/json")

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
				// read and log a small part of body for debugging
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				resp.Body.Close()
				log.Printf("discord: rate limited (429). Retry-After=%s; body=%s", ra, strings.TrimSpace(string(b)))
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
				// read part of body for debug
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
				resp.Body.Close()
				log.Printf("discord: server error %d, body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
				backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
				time.Sleep(backoff)
				continue
			}

			// other status codes (2xx success or 4xx client error) - do not retry
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
				resp.Body.Close()
				log.Printf("discord: unexpected status %d. body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
			} else {
				resp.Body.Close()
			}
			break
		}

		if err != nil {
			log.Printf("discord: request failed: %v", err)
			return
		}
		// if we exhausted retries, log and continue to next batch
		if attempt > maxRetries {
			log.Printf("discord: send failed after %d attempts", maxRetries)
		}
	}
}
