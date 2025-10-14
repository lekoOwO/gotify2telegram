# Gotify 2 Telegram (and Discord)
This Gotify plugin forwards received messages to Telegram and/or Discord.

## Prerequisite
- A Telegram bot, bot token, and chat ID from bot conversation. You can get that information by following this [blog](https://medium.com/linux-shots/setup-telegram-bot-to-get-alert-notifications-90be7da4444).
- Golang, Docker, wget (If you want to build the binary from source).

## Installation
* **By shared object**

    1. Get the compatible shared object from [release](https://github.com/lekoOwO/gotify2telegram/releases).

    2. Put it into Gotify plugin folder.

    3. Set secrets via environment variables (List of mandatory secrets is in [Appendix](#appendix)).

    4. Restart gotify.

    5. Config the plugin.

* **Build from source**

    1. Change GOTIFY_VERSION in Makefile.

    2. Build the binary.

    ```
    make build
    ```

    3. Follow instructions from step 2 in the shared object installation.

## Configuration

The configuration contains four keys: `clients`, `gotify_host`, `token` and `discord`.

This plugin supports sending to Telegram, Discord, or both. Each `SubClient` may independently enable Telegram and/or Discord.

### Clients

The `clients` configuration key describes which client(channel?) we are going to listen on and which telegram channel (and topic optionally!) we are forwarding the message to.

```yaml
clients:
  - app_id: "The Gotify App ID to be matched. use -1 for all-matching."
    telegram:
      chat_id: "ID of the telegram chat"
      token: "The bot token"
      thread_id: "Thread ID of the telegram topic. Leave it empty if we are not sending to a topic."
    discord:
      webhook_url: "https://discord.com/api/webhooks/..."
      username: "Optional per-client username (falls back to global discord defaults if empty)"
      avatar_url: "Optional per-client avatar URL (falls back to global defaults if empty)"
  - app_id: "Maybe the second Gotify Client Token, yay!"
    telegram:
      chat_id: "ID of the telegram chat"
      token: "The bot token"
      thread_id: "Thread ID of the telegram topic. Leave it empty if we are not sending to a topic."
```

### Global Discord defaults

You can set global Discord defaults (used when per-client username/avatar are empty):

```yaml
discord:
  username: "GotifyBot"
  avatar_url: "https://example.com/avatar.png"
```

### Gotify Host

The `gotify_host` configuration key should be set to `ws://YOUR_GOTIFY_IP` (depending on your setup, `ws://localhost:80` will likely work by default)

### Token

The `token` configuration key should be set to a valid token that can be created in the "Clients" tab.

## Troubleshooting
1. When only the Gotify dashboard receives your message, but not Telegram:

    If, when making the API call to get your bot's chat ID, no data is returned, you may need to change the bot's privacy settings.

    - In the BotFather chat, list your created bots and select the respective bot for which you want to change the Group Privacy setting.
    - Turn off the Group Privacy setting.
