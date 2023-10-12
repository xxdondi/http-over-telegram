package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elazarl/goproxy"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"
	"github.com/teris-io/shortid"
	"go.uber.org/zap"
)

type halfClosable interface {
	net.Conn
	CloseWrite() error
	CloseRead() error
}

func serializeReqBody(req *http.Request) string {
	var b = &bytes.Buffer{}              // holds serialized representation
	if err := req.Write(b); err != nil { // serialize request to HTTP/1.1 wire format
		panic(err)
	}
	return base64.RawStdEncoding.EncodeToString(b.Bytes())
}

func serializeRespBody(resp *http.Response) string {
	var b = &bytes.Buffer{}               // holds serialized representation
	if err := resp.Write(b); err != nil { // serialize request to HTTP/1.1 wire format
		panic(err)
	}
	return base64.RawStdEncoding.EncodeToString(b.Bytes())

}

func serializeReq(dir string, reqId string, host string, body string) string {
	return fmt.Sprintf("%s;%s;%s;%s", dir, reqId, host, body)
}

func deserializeReq(req string) (string, string, string, string) {
	split := strings.Split(req, ";")
	return split[0], split[1], split[2], split[3]
}

func readFromConn(conn net.Conn) ([]byte, error) {
	// make a temporary bytes var to read from the connection
	tmp := make([]byte, 1024)
	// make 0 length data bytes (since we'll be appending)
	data := make([]byte, 0)
	// keep track of full length read
	length := 0

	// loop through the connection stream, appending tmp to data
	for {
		// read to the tmp var
		conn.SetReadDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(tmp)
		if err != nil || n == 0 {
			break
		}

		// append read data to full data
		data = append(data, tmp[:n]...)

		// update total read var
		length += n
	}
	return data, nil
}

func main() {
	// Parse args
	var arg struct {
		Mode string
	}
	flag.StringVar(&arg.Mode, "mode", "none", "Mode to run in: 'enter' or 'exit'")
	flag.Parse()
	if arg.Mode == "none" {
		panic("Cannot run without --mode")
	}
	if arg.Mode != "enter" && arg.Mode != "exit" {
		panic(fmt.Sprintf("Invalid --mode value provided '%s', use 'enter' or 'exit'", arg.Mode))
	}

	// Parse env variables
	godotenv.Load()
	appId, err := strconv.Atoi(os.Getenv("APP_ID"))
	if err != nil {
		panic(err)
	}
	appHash := os.Getenv("APP_HASH")
	phone := os.Getenv("PHONE")
	password := os.Getenv("PASSWORD")
	chatIdInt, err := strconv.Atoi(os.Getenv("CHAT_ID"))
	if err != nil {
		panic(err)
	}
	chatId := int64(chatIdInt)

	// Set up tg client
	proxyClient := NewClient(appId, appHash, phone, password, chatId, arg.Mode)
	proxyClient.phone = phone
	log, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	defer log.Sync()

	requestWgs := make(map[string]*sync.WaitGroup)
	reqResp := make(map[string]string)

	var mainWg sync.WaitGroup
	mainWg.Add(2)

	msgsToSend := make(chan string)
	if arg.Mode == "enter" {

		// Set up proxy server
		proxyServer := goproxy.NewProxyHttpServer()
		// Run proxy server
		go func() {
			defer mainWg.Done()
			proxyServer.OnRequest(goproxy.ReqHostMatches(regexp.MustCompile(`^.*ycombinator.com.*$`))).DoFunc(
				func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
					reqId, err := shortid.Generate()
					if err != nil {
						panic(err)
					}
					requestWg := &sync.WaitGroup{}
					requestWg.Add(1)
					requestWgs[reqId] = requestWg
					body := serializeReqBody(req)
					reqStr := serializeReq("OUT", reqId, req.Host, body)
					msgsToSend <- reqStr
					log.Info("Waiting for response...", zap.String("reqId", reqId))
					requestWg.Wait()
					log.Info("Got response", zap.String("reqId", reqId))
					respStr, ok := reqResp[reqId]
					if !ok {
						panic("resp not found")
					}
					b, err := base64.RawStdEncoding.DecodeString(respStr)
					if err != nil {
						panic(err)
					}
					httpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(b)), req)
					if err != nil {
						panic(err)
					}
					return req, httpResp
				})
			if false {
				// HTTPS support (not fully working yet)
				proxyServer.OnRequest(goproxy.ReqHostMatches(regexp.MustCompile(`^.*:443.*$`))).
					HijackConnect(func(req *http.Request, client net.Conn, ctx *goproxy.ProxyCtx) {
						client.Write([]byte("HTTP/1.0 200 Connection established\r\n\r\n"))

						clientTCP, ok := client.(halfClosable)
						if !ok {
							panic("Client is not half closable")
						}

						bodyBuf, err := readFromConn(clientTCP)
						if err != nil {
							panic(err)
						}
						clientTCP.CloseRead()

						reqId, err := shortid.Generate()
						if err != nil {
							panic(err)
						}
						requestWg := &sync.WaitGroup{}
						requestWg.Add(1)
						requestWgs[reqId] = requestWg
						reqStr := serializeReq("OUT", reqId, req.Host, base64.RawStdEncoding.EncodeToString(bodyBuf))
						msgsToSend <- reqStr
						log.Info("Waiting for response...", zap.String("reqId", reqId))
						requestWg.Wait()
						log.Info("Got response", zap.String("reqId", reqId))
						respStr, ok := reqResp[reqId]
						if !ok {
							panic("resp not found")
						}
						b, err := base64.RawStdEncoding.DecodeString(respStr)
						if err != nil {
							panic(err)
						}
						clientTCP.Write(b)
						clientTCP.CloseWrite()
					})
			}
			log.Info("Starting proxy server on port %d", zap.Int("port", 8080))
			http.ListenAndServe(":8080", proxyServer)
		}()
	}

	// Run tg client
	go func() error {
		defer mainWg.Done()

		if arg.Mode == "exit" {
			proxyClient.OnChatMessage(func(ctx context.Context, msg tg.Message) error {
				log.Info("Got message", zap.String("text", msg.Message))
				dir, reqId, host, body := deserializeReq(msg.Message)
				if dir == "OUT" {
					decodedBody, err := base64.RawStdEncoding.DecodeString(body)
					if err != nil {
						panic(err)
					}
					if strings.Contains(host, ":443") {
						// HTTPS
						log.Info("Sending req", zap.String("reqId", reqId))
						conn, err := net.Dial("tcp", host)
						if err != nil {
							panic(err)
						}
						_, err = conn.Write(decodedBody)
						if err != nil {
							panic(err)
						}
						data, err := readFromConn(conn)
						if err != nil {
							panic(err)
						}
						conn.Close()
						log.Info("Sending message for req", zap.String("reqId", reqId))
						msgsToSend <- serializeReq("IN", reqId, host, base64.RawStdEncoding.EncodeToString(data))

					} else {
						// HTTP
						req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(decodedBody)))
						if err != nil {
							panic(err)
						}
						log.Info("Sending req", zap.String("reqId", reqId))
						client := &http.Client{}
						u, err := url.Parse("http://" + host + req.RequestURI)
						if err != nil {
							panic(err)
						}
						req.RequestURI = ""
						req.URL = u
						resp, err := client.Do(req)
						if err != nil {
							panic(err)
						}
						body := serializeRespBody(resp)
						log.Info("Sending message for req", zap.String("reqId", reqId))
						msgsToSend <- serializeReq("IN", reqId, host, body)
					}
				}
				return nil
			})
		} else {
			proxyClient.OnChatMessage(func(ctx context.Context, msg tg.Message) error {
				dir, reqId, _, body := deserializeReq(msg.Message)
				log.Info("Got message", zap.String("text", dir))
				if dir == "IN" {
					log.Info("Got IN message")
					requestWg, ok := requestWgs[reqId]
					if !ok {
						panic("requestWg not found")
					}
					delete(requestWgs, reqId)
					reqResp[reqId] = body
					requestWg.Done()
				}
				return nil
			})
		}
		return proxyClient.Run(context.Background())
	}()
	go func() error {
		for msg := range msgsToSend {
			proxyClient.SendMessage(context.Background(), msg)
		}
		return nil
	}()
	mainWg.Wait()
}
