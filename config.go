package main

import (
	"fmt"
)

type Telegram struct {
	ChatId	 string	 `yaml:"chat_id"`
	BotToken string	 `yaml:"token"`
	ThreadId string	 `yaml:"thread_id"`
}

type SubClient struct {
	GotifyClientToken	string	 `yaml:"token"`
	Telegram			Telegram `yaml:"telegram"`
}

// Config is user plugin configuration
type Config struct {
	Clients	   []SubClient	`yaml:"clients"`
	GotifyHost string		`yaml:"gotify_host"`
}

// DefaultConfig implements plugin.Configurer
func (c *Plugin) DefaultConfig() interface{} {
	return &Config{
		Clients: []SubClient{
			SubClient{
				GotifyClientToken: "ExampleToken",
				Telegram: Telegram{
					ChatId:	"-100123456789",
					BotToken: "YourBotTokenHere",
					ThreadId: "OptionalThreadIdHere",
				},
			},
		},
		GotifyHost: "ws://localhost:80",
	}
}

// ValidateAndSetConfig implements plugin.Configurer
func (c *Plugin) ValidateAndSetConfig(config interface{}) error {
	newConfig := config.(*Config)

	// Validate each SubClient in the Clients slice
	for i, client := range newConfig.Clients {
		if client.GotifyClientToken == "" {
			return fmt.Errorf("gotify client token is required for client %d", i)
		}
		if client.Telegram.BotToken == "" {
			return fmt.Errorf("telegram bot token is required for client %d", i)
		}
		if client.Telegram.ChatId == "" {
			return fmt.Errorf("telegram chat id is required for client %d", i)
		}
	}

	c.config = newConfig
	return nil
}