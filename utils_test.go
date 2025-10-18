package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatDiscordEmbeds_SplitsPlainText(t *testing.T) {
	// create a long message (~20k chars) where each line has a unique index suffix
	parts := make([]string, 0, 800)
	for i := 0; i < 800; i++ {
		parts = append(parts, fmt.Sprintf("ABCDEFGHIJKLMNOPQRSTUVWXYZ %d\n", i))
	}
	long := strings.Join(parts, "")
	msg := &GotifyMessage{
		Id:       1,
		Appid:    1,
		Message:  long,
		Title:    "Long Message",
		Priority: 1,
		Date:     "2025-10-19T00:00:00Z",
	}

	embeds := format_discord_embeds(msg)
	if len(embeds) <= 1 {
		t.Fatalf("expected multiple embeds for long message, got %d", len(embeds))
	}

	// Check embed sizes (title+desc+footer) under 6000
	footer := "Gotify Id: 1"
	for i, e := range embeds {
		total := len([]rune(e.Title)) + len([]rune(e.Description)) + len([]rune(footer))
		if total >= 6000 {
			t.Fatalf("embed %d too large: %d runes", i, total)
		}
	}

	// Ensure every original numbered line appears intact inside a single embed
	lines := strings.SplitAfter(long, "\n")
	for idx, line := range lines {
		if line == "" {
			continue
		}
		found := false
		for _, e := range embeds {
			if strings.Contains(e.Description, line) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("original numbered line %d not found intact in any embed: %q", idx, line)
		}
	}
}

func TestSendMsgToDiscord_Batching(t *testing.T) {
	// create many small embeds so batching is driven by maxEmbedsPerRequest
	total := 25
	embeds := make([]DiscordEmbed, total)
	for i := 0; i < total; i++ {
		embeds[i] = DiscordEmbed{
			Title:       fmt.Sprintf("title-%d", i+1),
			Description: strings.Repeat("x", 100),
			Color:       0x123456,
			Footer:      &DiscordEmbedFooter{Text: "footer"},
		}
	}

	var received [][]DiscordEmbed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p DiscordPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatalf("server decode error: %v", err)
		}
		received = append(received, p.Embeds)
		// respond with 204 No Content to indicate success
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	// call the function under test
	send_msg_to_discord(embeds, srv.URL, "tester", "")

	if len(received) == 0 {
		t.Fatalf("expected requests to be sent, got 0")
	}

	// expected number of requests = ceil(total / maxEmbedsPerRequest)
	expected := (total + maxEmbedsPerRequest - 1) / maxEmbedsPerRequest
	if len(received) != expected {
		t.Fatalf("expected %d requests, got %d", expected, len(received))
	}

	// check each payload obeys limits
	seen := 0
	for i, batch := range received {
		if len(batch) > maxEmbedsPerRequest {
			t.Fatalf("batch %d has %d embeds (exceeds max %d)", i, len(batch), maxEmbedsPerRequest)
		}
		batchRunes := 0
		for _, e := range batch {
			footerLen := 0
			if e.Footer != nil {
				footerLen = len([]rune(e.Footer.Text))
			}
			eRunes := len([]rune(e.Title)) + len([]rune(e.Description)) + footerLen
			batchRunes += eRunes
			if eRunes >= discordTotalMax {
				t.Fatalf("single embed in batch %d too large: %d runes", i, eRunes)
			}
			seen++
		}
		if batchRunes > discordTotalMax {
			t.Fatalf("batch %d total runes %d exceeds discordTotalMax %d", i, batchRunes, discordTotalMax)
		}
	}

	if seen != total {
		t.Fatalf("expected to see %d embeds across batches, saw %d", total, seen)
	}
}

func TestFormatDiscordEmbeds_PreservesCodeBlocks(t *testing.T) {
	// generate a long code block where each line has a unique index suffix
	parts := make([]string, 0, 2000)
	for i := 0; i < 2000; i++ {
		parts = append(parts, fmt.Sprintf("line_of_code(); // %d\n", i))
	}
	code := strings.Join(parts, "")
	body := "Some intro text\n```go\n" + code + "```\nSome outro"
	msg := &GotifyMessage{
		Id:       2,
		Appid:    1,
		Message:  body,
		Title:    "Code Message",
		Priority: 1,
		Date:     "2025-10-19T00:00:00Z",
	}

	embeds := format_discord_embeds(msg)
	if len(embeds) <= 1 {
		t.Fatalf("expected multiple embeds for long code block, got %d", len(embeds))
	}

	// Ensure each original code line appears intact inside a single embed's fenced block
	codeLines := strings.SplitAfter(code, "\n")

	// build a list of code-containing strings per embed
	embedCodeLines := make([][]string, len(embeds))
	for i, e := range embeds {
		parts := strings.Split(e.Description, "```")
		for j := 1; j+1 < len(parts); j += 2 {
			inner := parts[j]
			// remove language first line if present
			if idx := strings.Index(inner, "\n"); idx >= 0 {
				inner = inner[idx+1:]
			}
			// split inner into lines (preserve newline at end)
			ls := strings.SplitAfter(inner, "\n")
			embedCodeLines[i] = append(embedCodeLines[i], ls...)
		}
	}

	for idx, cl := range codeLines {
		if cl == "" {
			continue
		}
		found := false
		for _, ls := range embedCodeLines {
			for _, el := range ls {
				if el == cl {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatalf("code line %d not found intact in any embed's fenced block: %q", idx, cl)
		}
	}
}
