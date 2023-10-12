# http-over telegram

![ci status](https://github.com/xxdondi/http-over-telegram/actions/workflows/go.yml/badge.svg)

Proof-of-concept for transporting HTTP requests through Telegram servers to bypass internet censorship.

![diagram](./.github/img/http-over-telegram.drawio.svg)

> ⚠️ **Disclaimer**
> This is a proof of concept. It is not intended or ready for production use and does not provide seamless browsing experience

## How to run

### With Docker

Take `example.env` file, fill it with appropriate data:

```.env
# App ID and hash
# Get from https://my.telegram.org/ and NEVER share with anyone
APP_ID=123456
APP_HASH=abcdef123456
# Phone number
PHONE=+12345678
# TG password for 2FA
PASSWORD=112233
# Chat to use for exchange
# Create a group chat and add your bot to it
# use tg bot @username_to_id_bot to get ids
# OMIT THE MINUS SIGN
CHAT_ID=123
```

Build docker images for enter node and exit node:

```bash
# Enter node
docker build \
  --build-arg="MODE=enter" \
  -t hot-enter-node:latest .
  # (hot = http-over-telegram)

# Exit node
docker build \
  --build-arg="MODE=exit" \
  -t hot-exit-node:latest .
```

In order to keep session files permanent and not have to log in every time,
you should create a folder for session files and mount it to docker container.
It will contain your session files in directories unique by phone number and mode (enter/exit).

##### If you have 2FA enabled

First time you run a node, you have to use `-it` flag to log in to Telegram by entering a code
sent to your account. After that, you can use `-d` flag to the node background.

Once we have our session folder (e.g. `./session`) and we chose port `:8080`, we can run the enter node:

```bash
# Replace 0.0.0.0:8080 with your desired address and port, but map it to 8080 inside container
# Do not forget to mount session folder
docker run -p 0.0.0.0:8080:8080/tcp -v $(pwd)/session:/app/session -it hot-enter-node:latest
```

Running exit node:

```bash
# Do not forget to mount session folder
docker run -v $(pwd)/session:/app/session -it hot-exit-node:latest
```

There you have it, feel free to connect clients to HTTP proxy listening to `:8080` on your enter node.

### Run as developer

Use `Taskfile` commands:

```yml
task run-enter # runs an ENTER node
task run-exit # runs an EXIT node
task build # builds to ./bin
```

## Possible use cases

### Bypassing censorship

If Telegram is not blocked in your country, but internet censorship exists, you can use this tool to view blocked websites.

### Free internet

If you have a mobile data plan with unlimited Telegram traffic, you can use this tool to access any website for free.

### Security through obscurity

At least there definitely is a great deal of obscurity in this project. At the same time, who would think to look for HTTP traffic in Telegram, right?

## Known Limitations

### Rate Limiting

Telegram API has rather strict rate limits for sending messages.

That is why `gotd/ratelimit` and `gotd/floodwait` are used to mitigate this issue. You are not very likely (no guarantees) to get banned, you probably will just get a very slow connection.

> **Tip:** In order to get much better performance, you should use different Telegram accounts for enter node and exit node. This way, each client will have its own rate limit and you will be able to send more requests.

Another idea for sending less requests is request batching, however this way we might hit message length limit.

## TODO

- Finish HTTPS support
- Add WebSocket support (if possible?)
- Request batching to mitigate rate limiting
- Add tests
