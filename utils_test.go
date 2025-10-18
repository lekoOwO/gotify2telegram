package main

import (
    "strings"
    "testing"
)

func TestFormatDiscordEmbeds_SplitsPlainText(t *testing.T) {
    // create a long message (~20k chars)
    long := strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ\n", 800)
    msg := &GotifyMessage{
        Id:      1,
        Appid:   1,
        Message: long,
        Title:   "Long Message",
        Priority: 1,
        Date:    "2025-10-19T00:00:00Z",
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
}

func TestFormatDiscordEmbeds_PreservesCodeBlocks(t *testing.T) {
    // generate a long code block
    code := strings.Repeat("line_of_code();\n", 2000)
    body := "Some intro text\n```go\n" + code + "```\nSome outro"
    msg := &GotifyMessage{
        Id:      2,
        Appid:   1,
        Message: body,
        Title:   "Code Message",
        Priority: 1,
        Date:    "2025-10-19T00:00:00Z",
    }

    embeds := format_discord_embeds(msg)
    if len(embeds) <= 1 {
        t.Fatalf("expected multiple embeds for long code block, got %d", len(embeds))
    }

    // Collect code fragments by removing fences and newlines
    var collected strings.Builder
    for _, e := range embeds {
        // find fenced blocks in description
        parts := strings.Split(e.Description, "```")
        for j := 1; j+1 < len(parts); j += 2 {
            // parts[j] is inner content possibly starting with lang+newline
            inner := parts[j]
            // remove first line if it is a language tag
            if idx := strings.Index(inner, "\n"); idx >= 0 {
                inner = inner[idx+1:]
            }
            collected.WriteString(inner)
        }
    }

    got := collected.String()
    // remove trailing whitespace differences
    got = strings.TrimSpace(got)
    want := strings.TrimSpace(code)
    if !strings.Contains(got, "line_of_code();") {
        t.Fatalf("collected code does not look correct, length %d", len(got))
    }
    // ensure at least some of code preserved and concatenated
    if len(got) < len(want)/4 {
        t.Fatalf("collected code seems too short: got %d want ~%d", len(got), len(want))
    }
}
