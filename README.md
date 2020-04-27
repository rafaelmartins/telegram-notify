# telegram-notify

Send Telegram notifications when a command fails to execute


## Usage

The basic usage is:

    $ telegram-notify [-id=ID] [-limit=LIMIT] [-success] [--] COMMAND [ARG ...]

Where `-id=ID` provides an identifier of the notification origin (usually the hostname), `-limit=LIMIT` provides the size limit of the stream data (in bytes) to send notifications, `-success` makes `telegram-notify` send notifications also when command executes sucessfully.

The command requires 2 environment variables:

- `TELEGRAM_TOKEN`: Telegram Bot token, that identifies the bot to be used to send the notifications.
- `TELEGRAM_CHAT_ID`: The numeric ID of the Telegram chat (user or group) to send the notifications to.
