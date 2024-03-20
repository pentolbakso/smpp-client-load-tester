package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pentolbakso/smpp-go"
	"github.com/pentolbakso/smpp-go/pdu"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type MyLogger struct{}

func (l MyLogger) InfoF(msg string, params ...interface{}) {
	log.Info().Msgf(msg, params...)
}
func (l MyLogger) ErrorF(msg string, params ...interface{}) {
	log.Error().Msgf(msg, params...)
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	log.Info().Msgf("smpp-client-load-tester")

	msgId := 1
	bindConf := smpp.BindConf{
		SystemID:   "esme1",
		Password:   "password",
		SystemType: "smpp",
		Addr:       "127.0.0.1:5000",
	}

	sessionConf := smpp.SessionConf{
		SendWinSize: 10000,
		ReqWinSize:  10000,
		Logger:      MyLogger{},
		SessionState: func(sessionID string, systemID string, state smpp.SessionState) {
		},
		Handler: smpp.HandlerFunc(func(ctx *smpp.Context) {
			log.Debug().Msgf("Received smpp command: %v", ctx.CommandID().String())

			switch ctx.CommandID() {
			case pdu.DeliverSmID:
				req, err := ctx.DeliverSm()
				if err != nil {
					log.Error().Msgf("Invalid PDU in context error: %+v", err)
					return
				}

				msgId++
				resp := req.Response(fmt.Sprintf("msgID_%d", msgId))
				log.Debug().Msgf("Sending DeliverSm Response %v", resp)
				if err := ctx.Respond(resp, pdu.StatusOK); err != nil {
					log.Error().Msgf("Server can't respond to the DeliverSmID request: %+v", err)
				}

			case pdu.UnbindID:
				req, err := ctx.Unbind()
				if err != nil {
					log.Error().Msgf("Invalid PDU in context error: %+v", err)
					return
				}
				resp := req.Response()
				if err := ctx.Respond(resp, pdu.StatusOK); err != nil {
					log.Error().Msgf("Server can't respond to the UnbindID request: %+v", err)
					return
				}
				ctx.Sess.Close()

			case pdu.EnquireLinkID:
				req, err := ctx.EnquireLink()
				if err != nil {
					log.Error().Msgf("Invalid PDU in context error: %+v", err)
					return
				}
				resp := req.Response()
				if err := ctx.Respond(resp, pdu.StatusOK); err != nil {
					log.Error().Msgf("Server can't respond to the EnquireLinkID request: %+v", err)
					return
				}

			default:
				if err := ctx.Respond(&pdu.GenericNack{}, pdu.StatusOK); err != nil {
					log.Error().Msgf("Server can't respond to the request: %+v", err)
				}
			}
		}),
	}

	startTaskTimer(1*time.Second, func() bool {
		log.Info().Msgf("Connecting to smpp server...%v", bindConf)
		session, err := smpp.BindTRx(sessionConf, bindConf)
		if err != nil {
			log.Error().Msgf("BindTRX failed error: %+v", err)
			session.Close()
			return false
		}

		time.Sleep(5 * time.Second)
		start := time.Now()
		for i := 0; i < 100; i++ {
			sm := &pdu.SubmitSm{
				SourceAddr:      "11111111",
				DestinationAddr: "22222222",
				ShortMessage:    "Hello from SMPP!",
			}
			resp, _, err := session.Send(context.Background(), sm)
			if err != nil {
				log.Error().Msgf("SubmitSM failed error: %+v", err)
			} else {
				log.Debug().Msgf("Recv response %s %+v", resp.CommandID(), resp)
			}
		}
		elapsed := time.Since(start).Milliseconds()
		log.Debug().Msgf("Time elapsed: %d ms\n", elapsed)
		return true
	})

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Println("[q] quit")
		fmt.Print("Command: ")
		scanner.Scan()
		command := strings.ToLower(scanner.Text())
		if command == "q" {
			fmt.Println("Exiting...")
			break // Exit the loop if the exit command is entered
		} else {
			fmt.Println("Unknown command: " + command)
		}
	}
}

func startTaskTimer(interval time.Duration, fn func() bool) chan bool {
	stop := make(chan bool, 1)
	ticker := time.NewTicker(interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				if fn() {
					stop <- true
				}
			case <-stop:
				return
			}
		}
	}()

	return stop
}
