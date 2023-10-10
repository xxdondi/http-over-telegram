package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/gotd/contrib/middleware/floodwait"
	"github.com/gotd/contrib/middleware/ratelimit"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/time/rate"
)

type TgProxyClient struct {
	goTdClient  *telegram.Client
	log         *zap.SugaredLogger
	floodWaiter *floodwait.Waiter
	dispatcher  *tg.UpdateDispatcher
	phone       string
	password    string
	chatId      int64
}

func sessionFolder(phone string) string {
	var out []rune
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			out = append(out, r)
		}
	}
	return "phone-" + string(out)
}

func codePrompt(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter code: ")
	code, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

func NewClient(appId int, appHash string, phone string, password string, chatId int64, sessionSuffix string) *TgProxyClient {
	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapcore.InfoLevel),
		Development:      true,
		Encoding:         "json",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	rawLog, err := config.Build()
	log := rawLog.Sugar()
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	sessionDir := filepath.Join("session", sessionFolder(phone)+"-"+sessionSuffix)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		panic(err)
	}

	// So, we are storing session information in current directory, under subdirectory "session/phone_hash"
	sessionStorage := &telegram.FileSessionStorage{
		Path: filepath.Join(sessionDir, "session.json"),
	}
	log.Info("Storage", zap.String("path", sessionDir))

	waiter := floodwait.NewWaiter().WithCallback(func(ctx context.Context, wait floodwait.FloodWait) {
		// Notifying about flood wait.
		log.Warn("Flood wait", zap.Duration("wait", wait.Duration))
		fmt.Println("Got FLOOD_WAIT. Will retry after", wait.Duration)
	})

	dispatcher := tg.NewUpdateDispatcher()
	opts := telegram.Options{
		SessionStorage: sessionStorage,
		UpdateHandler:  dispatcher,
		Middlewares: []telegram.Middleware{
			waiter,
			ratelimit.New(rate.Every(4*time.Second), 5),
		},
	}

	client := telegram.NewClient(appId, appHash, opts)

	return &TgProxyClient{
		goTdClient:  client,
		phone:       phone,
		password:    password,
		log:         log,
		floodWaiter: waiter,
		dispatcher:  &dispatcher,
		chatId:      chatId,
	}
}

func (c *TgProxyClient) OnChatMessage(f func(ctx context.Context, msg tg.Message) error) {
	chatIdStr := strconv.FormatInt(c.chatId, 10)
	c.dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, u *tg.UpdateNewMessage) error {
		m, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		if m.PeerID.String() == "PeerChat{ChatID:"+chatIdStr+"}" {
			f(ctx, *u.Message.(*tg.Message))
		}
		return nil
	})
}

func (c *TgProxyClient) Run(ctx context.Context) error {
	flow := auth.NewFlow(auth.Constant(c.phone, c.password, auth.CodeAuthenticatorFunc(codePrompt)), auth.SendCodeOptions{})
	if err := c.floodWaiter.Run(ctx, func(ctx context.Context) error {
		return c.goTdClient.Run(ctx, func(ctx context.Context) error {
			if err := c.goTdClient.Auth().IfNecessary(ctx, flow); err != nil {
				return errors.Wrap(err, "auth")
			}

			return telegram.RunUntilCanceled(ctx, c.goTdClient)
		})
	}); err != nil {
		panic(err)
	}
	return nil
}

func (c *TgProxyClient) SendMessage(ctx context.Context, msg string) error {
	sender := message.NewSender(c.goTdClient.API())
	peer := &tg.InputPeerChat{
		ChatID: c.chatId,
	}
	sender.To(peer).Text(ctx, msg)
	return nil
}
