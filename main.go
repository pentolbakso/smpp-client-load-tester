package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
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
	if debug {
		log.Info().Msgf(msg, params...)
	}
}
func (l MyLogger) ErrorF(msg string, params ...interface{}) {
	log.Error().Msgf(msg, params...)
}

var (
	serverAddr string
	systemID   string
	password   string
	systemType string
	timeout    int
	smsFile    string
	debug      bool
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.TimeOnly})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	log.Info().Msgf("smpp-client-load-tester")

	flag.StringVar(&serverAddr, "addr", "127.0.0.1:2775", "SMSC server address and port")
	flag.StringVar(&systemID, "systemid", "esme", "Identifies the ESME system")
	flag.StringVar(&password, "password", "password", "The password may be used by the SMSC to authenticate the ESME")
	flag.StringVar(&systemType, "systemtype", "smpp", "Identifies the type of ESME system")
	flag.IntVar(&timeout, "timeout", 10, "Smpp request timeout")
	flag.StringVar(&smsFile, "smsfile", "sms.csv", "Import sms data from this file")
	flag.BoolVar(&debug, "smppdebug", true, "Show smpp debug log")
	flag.Parse()

	msgId := 1
	bindConf := smpp.BindConf{
		SystemID:   systemID,
		Password:   password,
		SystemType: systemType,
		Addr:       serverAddr,
	}

	sessionConf := smpp.SessionConf{
		SendWinSize:   10000,
		ReqWinSize:    10000,
		WindowTimeout: time.Duration(timeout * int(time.Second)),
		Logger:        MyLogger{},
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

	startTaskTimer(5*time.Second, func() bool {
		log.Info().Msgf("Connecting to smpp server...%v", bindConf)
		session, err := smpp.BindTRx(sessionConf, bindConf)
		if err != nil {
			log.Error().Msgf("BindTRX failed error: %+v", err)
			if session != nil {
				session.Close()
			}
			return false
		}
		log.Info().Msgf("Connected to smpp server. Preparing sms..")

		time.Sleep(1 * time.Second)
		loadAndSendSms(session)

		return true
	})

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Press [q] to quit\n\n")
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

func loadAndSendSms(session *smpp.Session) {
	start := time.Now()

	log.Info().Msgf("Opening sms file...")
	file, err := os.Open(smsFile)
	if err != nil {
		log.Fatal().Msgf("Error: %s", err)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = 3
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatal().Msgf("Read file error : %v", err)
		return
	}
	log.Info().Msgf("Sms file loaded.")

	log.Info().Msgf("Sending %v sms...", len(records))
	for _, record := range records {
		sm := &pdu.SubmitSm{
			SourceAddr:      record[0],
			DestinationAddr: record[1],
			ShortMessage:    record[2],
		}
		_, _, err := session.Send(context.Background(), sm)
		if err != nil {
			log.Error().Msgf("SubmitSM failed: %+v", err)
		} else {
			// log.Debug().Msgf("Recv response %s %+v", resp.CommandID(), resp)
		}
	}
	log.Info().Msgf("Sending sms done")

	// for i := 0; i < 100; i++ {
	// 	sm := &pdu.SubmitSm{
	// 		SourceAddr:      "11111111",
	// 		DestinationAddr: "22222222",
	// 		ShortMessage:    "Hello from SMPP!",
	// 	}
	// 	resp, _, err := session.Send(context.Background(), sm)
	// 	if err != nil {
	// 		log.Error().Msgf("SubmitSM failed error: %+v", err)
	// 	} else {
	// 		log.Debug().Msgf("Recv response %s %+v", resp.CommandID(), resp)
	// 	}
	// }

	elapsed := time.Since(start).Milliseconds()
	log.Info().Msgf("Time elapsed: %d ms\n", elapsed)
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
