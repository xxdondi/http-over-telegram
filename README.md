## http-over telegram

![ci status](https://github.com/xxdondi/http-over-telegram/actions/workflows/go.yml/badge.svg)

Proof-of-concept for transporting HTTP requests through Telegram servers to bypass internet censorship.

![diagram](./.github/img/http-over-telegram.drawio.svg)

> ⚠️ **Disclaimer**
> This is a proof of concept. It is not intended or ready for production use and does not provide seamless browser experience i.e. no YouTube. Browsing simple website

#### Running with Docker

Clone the repository and create "session" folder.
Fill .env file with your Telegram API credentials.

Build a docker image for an enter node and an exit node:

```bash
# Enter node
# (hot = http-over-telegram)
docker build --build-arg="MODE=enter" -t hot-enter-node:latest .

# Exit node
docker build --build-arg="MODE=exit" -t hot-exit-node:latest .
```

Running enter node:

```bash
docker run -p 0.0.0.0:8080:8080/tcp -v $(pwd)/session:/app/session -it hot-enter-node:latest
```

Running exit node:

```bash
docker run -v $(pwd)/session:/app/session -it hot-exit-node:latest
```

There you have it, feel free to connect clients to HTTP proxy listening to `:8080` on your enter node.

#### Running without Docker

Use `Taskfile` commands:

```yml
task run-enter # runs an ENTER node
task run-exit # runs an EXIT node
task build # builds to ./bin
```

#### Possible use cases

##### Bypassing censorship

If Telegram is not blocked in your country, but internet censorship exists, you can use this tool to view blocked websites.

##### Free internet

If you have a mobile data plan with unlimited Telegram traffic, you can use this tool to access any website for free.

#### Known Limitations

##### Rate Limiting

Telegram API has rather strict rate limits for sending messages.

That is why `gotd/ratelimit` and `gotd/floodwait` are used to mitigate this issue. You are not very likely (no guarantees) to get banned, you probably will just get a very slow connection.

> **Tip:** In order to get much better performance, you should use different Telegram accounts for enter node and exit node. This way, each client will have its own rate limit and you will be able to send more requests.

Another idea for sending less requests is request batching, however this way we might hit message length limit.

#### TODO

- Finish HTTPS support
- Add WebSocket support (if possible?)
- Request batching to mitigate rate limiting
- Add tests
