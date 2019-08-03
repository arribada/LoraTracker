package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"github.com/pkg/errors"
	"log"
	"strings"
	"time"

	"github.com/adrianmo/go-nmea"
	"github.com/calvernaz/rak811"
	"github.com/tarm/serial"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	respChan, err := enableGPS()
	if err != nil {
		log.Fatal("failed to enable gps err:", err)
	}

	lora, err := newLoraConnection()
	if err != nil {
		log.Fatal("failed to create lora connection err:", err)
	}

	for {
		dataGPS := <-respChan
		dataLora := hex.EncodeToString([]byte(dataGPS.Time.String()))
		fmt.Println("sent data", dataLora, dataGPS)

		fmt.Println(lora)

		resp, err := lora.Send("0,1," + dataLora)
		if err != nil || resp != "OK" {
			log.Fatalf("failed to send data err:%v resp:%v \n", err, resp)
		}
	}
}

func enableGPS() (chan nmea.RMC, error) {
	c := &serial.Config{Name: "/dev/ttyUSB0", Baud: 9600, ReadTimeout: 3000 * time.Second}
	s, err := serial.OpenPort(c)
	if err != nil {
		return nil, errors.Wrap(err, "enable port")
	}

	reader := bufio.NewReader(s.Reader())

	// Full ref: https://cdn-shop.adafruit.com/datasheets/PMTK_A08.pdf
	// Turn on just minimum info (RMC only, location):
	command := "PMTK314,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0"
	s.Write([]byte("$" + command + "*" + nmea.XORChecksum(command) + "\r\n"))

	if !gerMTKAck(314, reader) {
		return nil, errors.New("no cmd ack")
	}

	// Set update rate to once every 10 second (10hz).
	command = "PMTK220,10000"
	s.Write([]byte("$" + command + "*" + nmea.XORChecksum(command) + "\r\n"))
	if !gerMTKAck(220, reader) {
		return nil, errors.New("no cmd ack")
	}

	respChan := make(chan nmea.RMC)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				log.Fatal(err)
			}
			parsed, err := nmea.Parse(strings.TrimSpace(line))
			if err != nil {
				log.Fatal(err)
			}
			if parsed.DataType() == nmea.TypeRMC {
				data := parsed.(nmea.RMC)
				respChan <- data
			}
		}
	}()

	return respChan, nil
}

func newLoraConnection() (*rak811.Lora, error) {
	cfg := &serial.Config{
		Name:        "/dev/ttyAMA0",
		ReadTimeout: 20000 * time.Millisecond,
	}
	lora, err := rak811.New(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "create rak811 instance")
	}
	log.Println("lora module initialized")

	if err := lora.HardReset(); err != nil {
		return nil, errors.Wrap(err, "reset module")
	}
	log.Println("lora module reset")


	resp, err := lora.SetMode(0)
	if err != nil{
		return nil, errors.Wrapf(err, "set lora mod")
	}
	log.Println("lora module mode set",resp)

	resp, err = lora.SetConfig("nwks_key:01020304050607080910111213141516&dev_eui:3038383664388108&app_key:01020304050607080910111213141516&app_eui:0000010000000000")
	if err != nil{
		return nil, errors.Wrapf(err, "set lora config")
	}
	log.Println("lora module config set",resp)


	resp, err = lora.JoinOTAA()
	if err != nil {
		return nil, errors.Wrapf(err, "lora join")
	}
	log.Println("lora module joined",resp)

	return lora, nil
}

// gerCmdAck reads untill it gets an ack for the initial setup command or
// untill it reached a reader error.
// This ic because the module might be currenlty active so
// might receive another response before the ack recponse.
func gerMTKAck(cmdID int, reader *bufio.Reader) bool {
	var ok bool
	for x := 0; x < 20; x++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		resp, err := nmea.Parse(strings.TrimSpace(line))
		if resp.TalkerID() == nmea.TypeMTK {
			d := resp.(nmea.MTK)
			if d.Cmd == cmdID && d.Flag == 3 {
				ok = true
				break
			}
		}
	}
	return ok
}
