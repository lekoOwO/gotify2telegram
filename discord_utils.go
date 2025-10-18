package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Embed building helpers (moved from utils.go)
// ---------------------------------------------------------------------------

// Discord field limits (rune counts / conservative defaults)
const (
	discordTotalMax  = 6000
	discordDescMax   = 4096
	discordTitleMax  = 256
	discordFooterMax = 2048
	overheadMargin   = 200
	// maximum embeds per single Discord webhook request
	maxEmbedsPerRequest = 10
)

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
	// safety: cap title/footer to their maxes for calculation
	titleLen := len([]rune(title))
	if titleLen > discordTitleMax {
		titleLen = discordTitleMax
	}
	footerLen := len([]rune(footer))
	if footerLen > discordFooterMax {
		footerLen = discordFooterMax
	}

	// compute remaining budget then cap to description max
	allowed := discordTotalMax - titleLen - footerLen - overheadMargin
	if allowed > discordDescMax {
		allowed = discordDescMax
	}
	if allowed < 200 {
		allowed = 200
	}
	return allowed
}

// truncateRunes returns the first n runes of s (or s if shorter)
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// balanceFences ensures that triple-backtick fences are closed when a description
// has been truncated. It appends closing backticks if necessary and also
// fixes partial fence suffixes.
func balanceFences(s string) string {
	// if odd number of fences, append closing fence
	if strings.Count(s, "```")%2 != 0 {
		// ensure we don't leave a partial fence at the end
		if strings.HasSuffix(s, "``") {
			s += "`"
		} else if strings.HasSuffix(s, "`") {
			s += "``"
		} else {
			s += "\n```"
		}
	}
	return s
}

// chooseSplitAtNewline returns an end index inside r starting at 'i' with
// capacity 'cap'. It prefers the last newline inside the window so we split
// on line boundaries when possible (improves readability of embeds).
func chooseSplitAtNewline(r []rune, i int, cap int) int {
	end := i + cap
	if end >= len(r) {
		return len(r)
	}
	// search backwards for last newline within [i, end)
	for j := end; j > i; j-- {
		if r[j-1] == '\n' {
			return j
		}
	}
	// no newline found, fall back to end
	return end
}

// parse message into segments of code blocks and plain text
type segment struct {
	isCode     bool
	lang, text string
}

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
	// flush appends a built embed using the current accumulator.
	flush := func() {
		t := ""
		if hasTitle {
			t = title
		}
		// truncate fields to per-field limits to avoid Discord 400 errors
		t = truncateRunes(t, discordTitleMax)
		footer := truncateRunes(footerText, discordFooterMax)
		desc := truncateRunes(string(cur), discordDescMax)
		// ensure backtick fences are balanced after truncation
		desc = balanceFences(desc)
		embeds = append(embeds, DiscordEmbed{Title: t, Description: desc, Color: color, Timestamp: "", Footer: &DiscordEmbedFooter{Text: footer}})
		hasTitle = false
		cur = []rune{}
		allowed = allowedDescFor("", footer)
	}

	for _, s := range segs {
		if s.isCode {
			opener := "```"
			if s.lang != "" {
				opener += s.lang + "\n"
			} else {
				opener += "\n"
			}
			closer := "```"
			r := []rune(s.text)
			i := 0
			for i < len(r) {
				remaining := allowed - len(cur) - len([]rune(opener)) - len([]rune(closer))
				if remaining <= 0 {
					flush()
					continue
				}
				// prefer to split at newline boundaries inside code
				end := chooseSplitAtNewline(r, i, remaining)
				take := end - i
				if take <= 0 {
					// nothing progress (shouldn't happen), force one rune to avoid infinite loop
					take = 1
				}
				frag := string(r[i : i+take])
				cur = append(cur, []rune(opener+frag+"\n"+closer)...)
				i += take
				if i < len(r) {
					flush()
				}
			}
		} else {
			r := []rune(s.text)
			i := 0
			for i < len(r) {
				remaining := allowed - len(cur)
				if remaining <= 0 {
					flush()
					continue
				}
				// prefer to split on newline boundaries for plain text as well
				end := chooseSplitAtNewline(r, i, remaining)
				take := end - i
				if take <= 0 {
					take = 1
				}
				cur = append(cur, r[i:i+take]...)
				i += take
				if i < len(r) {
					flush()
				}
			}
		}
	}
	if len(cur) > 0 || len(embeds) == 0 {
		flush()
	}
	return embeds
}

// DiscordPayload represents a Discord webhook payload that can include embeds.
type DiscordPayload struct {
	Username  string         `json:"username,omitempty"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Content   string         `json:"content,omitempty"`
	Embeds    []DiscordEmbed `json:"embeds,omitempty"`
}

type DiscordEmbed struct {
	Title       string              `json:"title,omitempty"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
	Footer      *DiscordEmbedFooter `json:"footer,omitempty"`
}

type DiscordEmbedFooter struct {
	Text string `json:"text,omitempty"`
}

// format_discord_embeds builds one or more embeds from GotifyMessage.
// It relies on smaller helpers in utils.go (parseSegments, buildEmbedsFromSegments).
func format_discord_embeds(msg *GotifyMessage) []DiscordEmbed {
	title := msg.Title
	footerText := fmt.Sprintf("Gotify Id: %d", msg.Id)
	segs := parseSegments(msg.Message)
	embeds := buildEmbedsFromSegments(title, segs, footerText, msg.Priority)

	for i := range embeds {
		embeds[i].Timestamp = msg.Date
		if len(embeds) > 1 {
			embeds[i].Footer = &DiscordEmbedFooter{Text: fmt.Sprintf("Gotify Id: %d (#%d)", msg.Id, i+1)}
		}
	}
	return embeds
}

// embedRunes computes the rune-count footprint of an embed's title+description+footer
func embedRunes(e DiscordEmbed) int {
	footerLen := 0
	if e.Footer != nil {
		footerLen = len([]rune(e.Footer.Text))
	}
	return len([]rune(e.Title)) + len([]rune(e.Description)) + footerLen
}

// batchEmbeds splits a list of embeds into batches so each batch has at most
// maxEmbedsPerRequest embeds and the total runes across the batch do not
// exceed discordTotalMax. Oversized single embeds are emitted alone.
func batchEmbeds(embeds []DiscordEmbed) [][]DiscordEmbed {
	var batches [][]DiscordEmbed
	i := 0
	for i < len(embeds) {
		batch := make([]DiscordEmbed, 0, maxEmbedsPerRequest)
		batchRunes := 0
		for i < len(embeds) && len(batch) < maxEmbedsPerRequest {
			e := embeds[i]
			eRunes := embedRunes(e)

			if eRunes > discordTotalMax {
				log.Printf("discord: single embed exceeds total limit (%d runes) — title+desc+footer=%d", discordTotalMax, eRunes)
				if len(batch) > 0 {
					// finish current batch first
					break
				}
				// emit oversized embed alone
				batch = append(batch, e)
				i++
				break
			}

			if batchRunes+eRunes > discordTotalMax {
				if len(batch) == 0 {
					// nothing fits in empty batch (shouldn't happen if eRunes <= discordTotalMax)
					batch = append(batch, e)
					batchRunes += eRunes
					i++
				}
				break
			}

			batch = append(batch, e)
			batchRunes += eRunes
			i++
		}
		if len(batch) > 0 {
			batches = append(batches, batch)
		}
	}
	return batches
}

// sendPayload sends a marshaled payload to the webhook with retries and
// handles rate-limiting (429) and server errors.
func sendPayload(client *http.Client, webhookURL string, payloadBytes []byte) error {
	var resp *http.Response
	var req *http.Request
	var err error
	maxRetries := 5

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err = http.NewRequest("POST", webhookURL, bytes.NewReader(payloadBytes))
		if err != nil {
			log.Println("discord: create request failed")
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			backoff := time.Duration(1<<attempt) * 200 * time.Millisecond
			time.Sleep(backoff)
			continue
		}

		// rate limit handling
		if resp.StatusCode == 429 {
			ra := resp.Header.Get("Retry-After")
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			log.Printf("discord: rate limited (429). Retry-After=%s; body=%s", ra, strings.TrimSpace(string(b)))
			var wait time.Duration
			if ra != "" {
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

		// server error handling
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()
			log.Printf("discord: server error %d, body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			time.Sleep(backoff)
			continue
		}

		// client error / unexpected
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			bodyStr := strings.TrimSpace(string(b))
			log.Printf("discord: unexpected status %d. body=%s", resp.StatusCode, bodyStr)

			if resp.StatusCode >= 400 && resp.StatusCode < 500 {
				pb := payloadBytes
				previewLen := 200
				if len(pb) <= previewLen*2 {
					log.Printf("discord: payload (len=%d): %s", len(pb), string(pb))
				} else {
					prefix := string(pb[:previewLen])
					suffix := string(pb[len(pb)-previewLen:])
					log.Printf("discord: payload len=%d; prefix=%s ... suffix=%s", len(pb), prefix, suffix)
				}
			}
		} else {
			resp.Body.Close()
		}
		break
	}

	if err != nil {
		log.Printf("discord: request failed: %v", err)
		return err
	}
	return nil
}

// send_msg_to_discord posts embeds to a Discord webhook. It might send multiple requests if given multiple embeds.
func send_msg_to_discord(embeds []DiscordEmbed, webhookURL string, username string, avatarURL string) {
	if webhookURL == "" {
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}

	batches := batchEmbeds(embeds)

	for _, batch := range batches {
		payload := DiscordPayload{
			Username:  username,
			AvatarURL: avatarURL,
			Embeds:    batch,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Println("discord: create json failed")
			return
		}

		if err := sendPayload(client, webhookURL, payloadBytes); err != nil {
			// sendPayload already logs; stop on persistent error
			return
		}
	}
}
