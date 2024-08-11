# Gotify 2 Telegram
This Gotify plugin forwards all received messages to Telegram through the Telegram bot.

## Prerequisite
- A Telegram bot, bot token, and chat ID from bot conversation. You can get that information by following this [blog](https://medium.com/linux-shots/setup-telegram-bot-to-get-alert-notifications-90be7da4444).
- Golang, Docker, wget (If you want to build the binary from source).

## Installation
* **By shared object**

    1. Get the compatible shared object from [release](https://github.com/anhbh310/gotify2telegram/releases).

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

The configuration contains two keys: `clients` and `gotify_host`.

### Clients

The `clients` configuration key describes which client(channel?) we are going to listen on and which telegram channel (and topic optionally!) we are forwarding the message to.

```yaml
clients:
  - token: "The Gotify Client Token"
    telegram:
      chat_id: "ID of the telegram chat"
      token: "The bot token"
      thread_id: "Thread ID of the telegram topic. Leave it empty if we are not sending to a topic."
  - token: "Maybe the second Gotify Client Token, yay!"
    telegram:
      chat_id: "ID of the telegram chat"
      token: "The bot token"
      thread_id: "Thread ID of the telegram topic. Leave it empty if we are not sending to a topic."
```

### Gotify Host

The `gotify_host` configuration key should be set to `ws://YOUR_GOTIFY_IP` (depending on your setup, `ws://localhost:80` will likely work by default)

## Troubleshooting
1. When only the Gotify dashboard receives your message, but not Telegram:

    If, when making the API call to get your bot's chat ID, no data is returned, you may need to change the bot's privacy settings.

    - In the BotFather chat, list your created bots and select the respective bot for which you want to change the Group Privacy setting.
    - Turn off the Group Privacy setting.
