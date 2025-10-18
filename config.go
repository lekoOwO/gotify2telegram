package main

import (
	"fmt"
)

type Telegram struct {
	ChatId   string `yaml:"chat_id"`
	BotToken string `yaml:"token"`
	ThreadId string `yaml:"thread_id"`
}

type DiscordDefaults struct {
	Username  string `yaml:"username"`
	AvatarURL string `yaml:"avatar_url"`
}

type Discord struct {
	WebhookURL string `yaml:"webhook_url"`
	DiscordDefaults
}

type SubClient struct {
	AppId    int      `yaml:"app_id"`
	Telegram Telegram `yaml:"telegram"`
	Discord  Discord  `yaml:"discord"`
}

// Config is user plugin configuration
type Config struct {
	Clients           []SubClient     `yaml:"clients"`
	GotifyHost        string          `yaml:"gotify_host"`
	GotifyClientToken string          `yaml:"token"`
	DiscordDefaults   DiscordDefaults `yaml:"discord"`
}

// DefaultConfig implements plugin.Configurer
func (c *Plugin) DefaultConfig() interface{} {
	return &Config{
		Clients: []SubClient{
			SubClient{
				AppId: 0,
				Telegram: Telegram{
					ChatId:   "-100123456789",
					BotToken: "YourBotTokenHere",
					ThreadId: "OptionalThreadIdHere",
				},
				Discord: Discord{
					WebhookURL: "",
					DiscordDefaults: DiscordDefaults{
						Username:  "DefaultUsername",
						AvatarURL: "DefaultAvatarURL",
					},
				},
			},
		},
		DiscordDefaults: DiscordDefaults{
			Username:  "DefaultUsername",
			AvatarURL: "DefaultAvatarURL",
		},
		GotifyHost:        "ws://localhost:80",
		GotifyClientToken: "ExampleToken",
	}
}

// ValidateAndSetConfig implements plugin.Configurer
func (c *Plugin) ValidateAndSetConfig(config interface{}) error {
	newConfig := config.(*Config)

	if newConfig.GotifyClientToken == "ExampleToken" {
		return fmt.Errorf("gotify client token is required")
	}
	for i, client := range newConfig.Clients {
		if client.AppId == 0 {
			return fmt.Errorf("gotify app id is required for client %d", i)
		}
		// Require at least one destination: Telegram or Discord
		if client.Telegram.BotToken == "" && client.Discord.WebhookURL == "" {
			return fmt.Errorf("either telegram or discord must be configured for client %d", i)
		}

		if client.Telegram.BotToken != "" {
			if client.Telegram.ChatId == "" {
				return fmt.Errorf("telegram chat id is required for client %d", i)
			}
		}

		if client.Discord.WebhookURL != "" {
			// very basic validation
			if len(client.Discord.WebhookURL) < 8 {
				return fmt.Errorf("discord webhook url seems invalid for client %d", i)
			}
		}
	}

	c.config = newConfig
	return nil
}
