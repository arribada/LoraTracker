package main

import (
	"bufio"
	"encoding/hex"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/adrianmo/go-nmea"
	"github.com/arribada/rak811"
	"github.com/tarm/serial"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	debug := os.Getenv("DEBUG") != ""
	if debug {
		log.Println("displaying debug logs")
	}

	h := os.Getenv("HDOP")
	HDOP := float64(-1)
	if h != "" {
		h, err := strconv.ParseFloat(h, 64)
		if err != nil {
			log.Fatalf("uanble to parse HDOP err:%v", err)
		}
		HDOP = h
		log.Println("set to skip gps data with  accuracy below HDOP:", HDOP)
	}
	appKey := os.Getenv("app_key")
	if appKey == "" {
		log.Fatal("missing app_key env variable")
	}
	if len(appKey) != 32 {
		log.Fatalf("app_kee should be 32 char long, app key length:%v", len(appKey))
	}
	devEUI := os.Getenv("dev_eui")
	if appKey == "" {
		log.Fatal("missing dev_eui env variable")
	}
	if len(devEUI) != 16 {
		log.Fatalf("dev_eui should be 16 char long, app key length:%v", len(devEUI))
	}

	if debug {
		log.Println("enabling gps module")
	}
	respChan, err := startGPS(debug, HDOP)
	if err != nil {
		log.Fatal("failed to enable gps err:", err)
	}

	if debug {
		log.Println("enabling lora module")
	}
	lora, err := newLoraConnection(devEUI, appKey, debug)
	if err != nil {
		log.Fatal("failed to create lora connection err:", err)
	}

	attempt := 1
	fake := "GPGGA,215147.000,4226.8739,N,02724.9090,E,1,10,1.00,28.8,M,37.8,M,,"
	parsed, err := nmea.Parse("$" + fake + "*" + nmea.Checksum(fake))
	// Set an initial GPS to the fake ones and than will be overwritten by the first available GPS coordinates.
	var lastGPSdata nmea.GGA = parsed.(nmea.GGA)
	for {
		dataGPS := <-respChan

		// Send only GPS data if it is valid.
		if dataGPS.FixQuality == nmea.Invalid {
			if os.Getenv("SEND_FAKE_GPS") == "" {
				if debug {
					log.Println("skipped sending an invalid data:", dataGPS)
				}
				continue
			}

			if err != nil {
				log.Println(err)
				continue
			}
			dataGPS = lastGPSdata
			log.Println("the GPS returned invalid data so sending a fake one")
		}
		// When HDOP precision is set,
		// skip sending anything below the threshold.
		if HDOP != -1 {
			if dataGPS.HDOP >= HDOP {
				if debug {
					log.Printf("skip sending low accuracy GPS accuracy threshold:%v  data:%v", HDOP, dataGPS)
				}
				continue
			}
		}

		lastGPSdata = dataGPS

		dataLora := hex.EncodeToString([]byte(strconv.FormatFloat(dataGPS.Latitude, 'f', -1, 64) + "," + strconv.FormatFloat(dataGPS.Longitude, 'f', -1, 64)))
		if debug {
			log.Printf("%v:trying to send gps data:%v \n", attempt, dataGPS)
		}
		resp, err := lora.Send("0,1," + dataLora)
		if err != nil {
			log.Println("failed to send data err:", err)
			// Attempt to register again.
			log.Println(attempt, ":registration retry")
			attempt++
			lora, _ = newLoraConnection(devEUI, appKey, debug)
			continue
		}

		signal, err := lora.Signal()
		if err != nil {
			log.Println("failed to get last packet signal info err:", err)
		}

		if resp == rak811.STATUS_TX_COMFIRMED {
			log.Println("STATUS_TX_COMFIRMED response received, signal:", signal)
			attempt = 1
		}
		if resp == rak811.STATUS_TX_UNCOMFIRMED {
			log.Println("STATUS_TX_UNCOMFIRMED response received, signal:", signal)
			attempt = 1
		}
	}
}

func startGPS(debug bool, HDOP float64) (chan nmea.GGA, error) {
	respChan := make(chan nmea.GGA)
	go func() {
		reader, err := setupGPS(debug)
		if err != nil {
			log.Fatalf("initial gps module setup err:%v", err)
		}
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				log.Println("reading gps serial err:", err)
				reader, err = setupGPS(debug)
				if err != nil {
					log.Fatalf("gps setup retry after a failed read err:%v", err)
				}
			}
			line = strings.TrimSpace(line)
			parsed, err := nmea.Parse(line)
			if err != nil {
				log.Println("unable to parse GPS response line:", line, " err:", err)
				continue
			}
			if parsed.DataType() == nmea.TypeGGA {
				dataGPS := parsed.(nmea.GGA)
				select {
				case respChan <- dataGPS: // Don't block when the reciver is not ready.
				default:
				}
			}
		}
	}()

	return respChan, nil
}

func setupGPS(debug bool) (*bufio.Reader, error) {
	c := &serial.Config{Name: "/dev/ttyUSB0", Baud: 9600, ReadTimeout: 3000 * time.Second}
	s, err := serial.OpenPort(c)
	if err != nil {
		return nil, errors.Wrap(err, "enable port")
	}

	reader := bufio.NewReader(s)

	// Full ref: https://cdn-shop.adafruit.com/datasheets/PMTK_A08.pdf
	// Turn on GGA:
	command := "PMTK314,0,0,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0"
	s.Write([]byte("$" + command + "*" + nmea.Checksum(command) + "\r\n"))

	if !gerMTKAck(debug, 314, reader) {
		return nil, errors.New("no cmd ack")
	}

	// Set update rate to once every 10 second (10hz).
	command = "PMTK220,10000"
	s.Write([]byte("$" + command + "*" + nmea.Checksum(command) + "\r\n"))
	if !gerMTKAck(debug, 220, reader) {
		return nil, errors.New("no cmd ack")
	}
	return reader, nil
}

func newLoraConnection(devEUI, appKey string, debug bool) (*rak811.Lora, error) {
	cfg := &serial.Config{
		Name: "/dev/ttyAMA0", // Inside docker /dev/serial0 is not available even in priviliged.
	}
	lora, err := rak811.New(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "create rak811 instance")
	}

	log.Println("lora module initialized")

	resp, err := lora.HardReset()
	if err != nil {
		return nil, errors.Wrap(err, "reset module")
	}
	log.Println("lora module reset resp:", resp)

	resp, err = lora.SetMode(0)
	if err != nil {
		return nil, errors.Wrapf(err, "set lora mod")
	}
	log.Println("lora module mode set resp:", resp)

	config := "pwr_level:7" + "&dev_eui:" + devEUI + "&app_key:" + appKey + "&app_eui:0000010000000000" + "&nwks_key:00000000000000000000000000000000"
	resp, err = lora.SetConfig(config)
	if err != nil {
		return nil, errors.Wrapf(err, "set lora config with:%v", config)
	}
	if debug {
		log.Print("lora module config set resp:", resp, " config:", config)
	}

	// Try to register undefinitely.
	// The lora gateway might be down so should keep trying and not exit.
	attempt := 1
	for {
		now := time.Now()
		if debug {
			log.Print("sending join request attempt:", attempt)
		}
		resp, err = lora.JoinOTAA()
		if err != nil {
			log.Println("Reseting the module due to a join request err:", err)
			newLoraConnection(devEUI, appKey, debug)
		}

		if resp == rak811.STATUS_JOINED_SUCCESS {
			log.Println("lora module joined, request duration:", time.Since(now))
			break
		}
		if debug {
			log.Print("unexpected gateway registration resp:", resp, " attempt:", attempt)
		}
		attempt++
	}
	return lora, nil
}

// gerCmdAck reads untill it gets an ack for the initial setup command or
// untill it reached a reader error.
// This ic because the module might be currenlty active so
// might receive another response before the ack recponse.
func gerMTKAck(debug bool, cmdID int64, reader *bufio.Reader) bool {
	var ok bool
	for x := 0; x < 20; x++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if debug {
			log.Println("gps cmd ack responce line:", line)
		}
		resp, err := nmea.Parse(strings.TrimSpace(line))
		if err != nil {
			log.Fatal("parsing the response:", line, "err:", err)
		}
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
