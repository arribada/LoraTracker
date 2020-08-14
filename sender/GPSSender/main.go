package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/adrianmo/go-nmea"
	"github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device"
	"github.com/arribada/LoraTracker/sender/GPSSender/pkg/rak811"
	"github.com/tarm/serial"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	debug := os.Getenv("DEBUG") != ""
	if debug {
		log.Println("with debug logs")
	}

	if f := os.Getenv("SEND_FREQ"); f != "" {
		freq, err := strconv.Atoi(f)
		if err == nil {
			log.Println("send frequency set to :", freq)
		} else {
			log.Println("couldn't parse the SEND_FREQ env variable:", err)
			os.Unsetenv("SEND_FREQ")
		}
	}

	h := os.Getenv("HDOP")
	HDOP := float64(-1)
	if h != "" {
		h, err := strconv.ParseFloat(h, 64)
		if err != nil {
			log.Fatalf("unable to parse HDOP err:%v", err)
		}
		HDOP = h
		log.Println("set to skip gps data with  accuracy below HDOP:", HDOP)
	}

	if debug {
		log.Println("enabling gps module")
	}
	gps, err := newGPS(debug)
	if err != nil {
		log.Fatal("failed to enable gps err:", err)
	}

	if debug {
		log.Println("enabling lora module")
	}
	lora, err := newLoraConnection(debug)
	if err != nil {
		log.Fatal("failed to create lora connection err:", err)
	}

	attempt := 1
	var fakeGPSdata = nmea.GGA{
		Latitude:  42.695109,
		Longitude: 23.360378,
	}
	if os.Getenv("SEND_FAKE_GPS") != "" {
		fakeData, err := device.Rpi(os.Getenv("SEND_FAKE_GPS"))
		if err == nil {
			fakeGPSdata.Latitude = fakeData.Lat
			fakeGPSdata.Longitude = fakeData.Lon
			if os.Getenv("DEBUG") != "" {
				log.Println("using coordinates from the fake enf var:", fakeData.Lat, fakeData.Lon)
			}
		} else if os.Getenv("DEBUG") != "" {
			log.Println("SEND_FAKE_GPS env didn't include valid coordinates to will the default ones in Bulgaria. err:", err)
		}
	}

	invalidCount := 0
	for {
		if invalidCount > 50 {
			log.Println("reseting the gps module for too many invalid gps fixes:", invalidCount)
			if err := gps.reset(); err != nil {
				log.Fatal(err)
			}
			gps, err = newGPS(debug)
			if err != nil {
				log.Fatal("failed to enable gps err:", err)
			}
			invalidCount = 0
		}

		dataGPS := <-gps.channel()
		// Send only GPS data if it is valid.
		if dataGPS.FixQuality == nmea.Invalid {
			invalidCount++
			if os.Getenv("SEND_FAKE_GPS") == "" {
				if debug {
					log.Printf("skipped sending an invalid data:%v invalid count:%v", dataGPS, invalidCount)
				}
				continue
			}
			dataGPS = fakeGPSdata
			log.Println("the GPS returned invalid data so sending a fake one")
		}
		// When HDOP precision is set,
		// skip sending anything below the threshold.
		if HDOP != -1 {
			invalidCount++
			if dataGPS.HDOP >= HDOP {
				if debug {
					log.Printf("skip sending low accuracy GPS accuracy data HDOP threshold:%v, current HDOP:%v, invalid count:%v", HDOP, dataGPS.HDOP, invalidCount)
				}
				continue
			}
		}
		invalidCount = 0

		// The amount of data that can be send is limited by region and dr.
		// If the received data is empty should increase the dr settings of the lora module.
		dataLora := fmt.Sprintf("%.6f", dataGPS.Latitude) + "," + fmt.Sprintf("%.6f", dataGPS.Longitude)
		if os.Getenv("SINGLE_POINTS") != "" {
			dataLora += ",s"
		}
		if debug {
			log.Printf("%v:trying to send gps GGA:%v lora:%v encoded:%v\n", attempt, dataGPS, dataLora, hex.EncodeToString([]byte(dataLora)))
		}
		resp, err := lora.Send("0,1," + hex.EncodeToString([]byte(dataLora)))
		if err != nil {
			log.Println("failed to send data err:", err)
			// Attempt to register again.
			log.Println(attempt, ":registration retry")
			attempt++
			lora, _ = newLoraConnection(debug)
			continue
		}
		if f := os.Getenv("SEND_FREQ"); f != "" {
			freq, err := strconv.Atoi(f)
			if err == nil {
				time.Sleep(time.Duration(freq) * time.Second)
			} else {
				log.Println("couldn't parse the SEND_FREQ env variable:", err)
			}
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

type gps struct {
	*serial.Port
	*bufio.Reader
	ch     chan nmea.GGA
	debug  bool
	closed chan struct{}
}

func newGPS(debug bool) (*gps, error) {
	gps := &gps{
		ch:     make(chan nmea.GGA),
		debug:  debug,
		closed: make(chan struct{}),
	}

	err := gps.setup()
	if err != nil {
		log.Fatalf("initial gps module setup err:%v", err)
	}

	go func() {
	loop:
		for {
			line, err := gps.ReadString('\n')
			select {
			case <-gps.closed:
				break loop
			default:
			}
			if err != nil {
				log.Fatal("reading gps serial err:", err)
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
				case gps.ch <- dataGPS: // Don't block when the reciver is not ready.
				default:
				}
			}
			time.Sleep(10 * time.Second)
		}
	}()

	return gps, nil

}

func (g *gps) channel() chan nmea.GGA {
	return g.ch
}

func (g *gps) reset() error {
	if err := g.close(); err != nil {
		log.Println("closing the gps port:", err)
	}
	err := ioutil.WriteFile("/sys/devices/platform/soc/20980000.usb/buspower", []byte("0"), 0644)
	if err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	err = ioutil.WriteFile("/sys/devices/platform/soc/20980000.usb/buspower", []byte("1"), 0644)
	if err != nil {
		return err
	}
	time.Sleep(1 * time.Second)
	return nil
}

func (g *gps) setup() error {
	if err := g.setupPort(); err != nil {
		return errors.Wrapf(err, "setup port")
	}
	// Drain the serial port from any previous commands.
	g.ReadString('\n')

	// Full ref: https://cdn-shop.adafruit.com/datasheets/PMTK_A08.pdf
	// Turn on GGA:
	command := "PMTK314,0,0,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0"
	g.Write([]byte("$" + command + "*" + nmea.Checksum(command) + "\r\n"))

	if !g.gerMTKAck(314) {
		return errors.New("no cmd ack")
	}

	// Set update rate to once every 1 seconds.
	command = "PMTK220,1000"
	g.Write([]byte("$" + command + "*" + nmea.Checksum(command) + "\r\n"))
	if !g.gerMTKAck(220) {
		return errors.New("no cmd ack")
	}
	return nil
}

func (g *gps) setupPort() error {
	portPath, err := selectPort()
	if err != nil {
		return errors.Wrapf(err, "selecting gps port")
	}
	if g.debug {
		log.Println("selected gps port:", portPath)
	}

	c := &serial.Config{Name: portPath, Baud: 9600, ReadTimeout: 3000 * time.Second}
	port, err := serial.OpenPort(c)
	if err != nil {
		return errors.Wrap(err, "enable port")
	}
	g.Port = port
	g.Reader = bufio.NewReader(port)
	return nil
}

func selectPort() (string, error) {
	portPath := ""
	for index := 0; index < 10; index++ {
		if _, err := os.Stat("/dev/ttyUSB0"); err == nil {
			portPath = "/dev/ttyUSB0"
		}
		if _, err := os.Stat("/dev/ttyUSB1"); err == nil {
			portPath = "/dev/ttyUSB1"
		}
		if portPath != "" {
			break
		}
		err := ioutil.WriteFile("/sys/devices/platform/soc/20980000.usb/buspower", []byte("1"), 0644)
		if err != nil {
			return "", err
		}
		time.Sleep(300 * time.Millisecond)
	}
	if portPath == "" {
		return "", errors.New("no gps usb device exists")
	}
	return portPath, nil
}

// gerCmdAck reads untill it gets an ack for the initial setup command or
// untill it reached a reader error.
// This ic because the module might be currenlty active so
// might receive another response before the ack recponse.
func (g *gps) gerMTKAck(cmdID int64) bool {
	var ok bool
	for x := 0; x < 20; x++ {
		line, err := g.ReadString('\n')
		if err != nil {
			break
		}
		if g.debug {
			log.Println("gps cmd ack response line:", line)
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

func (g *gps) close() error {
	close(g.closed)
	return g.Close()
}

func newLoraConnection(debug bool) (*rak811.Lora, error) {
	appKey := os.Getenv("APP_KEY")
	if appKey == "" {
		log.Fatal("missing APP_KEY env variable")
	}
	if len(appKey) != 32 {
		log.Fatalf("APP_KEY should be 32 char long, current length:%v", len(appKey))
	}
	devEUI := os.Getenv("DEV_EUI")
	if appKey == "" {
		log.Fatal("missing DEV_EUI env variable")
	}
	if len(devEUI) != 16 {
		log.Fatalf("DEV_EUI should be 16 char long, current length:%v", len(devEUI))
	}

	dataRate := os.Getenv("DATA_RATE")
	if dataRate == "" {
		dataRate = "1"
	}
	s, err := strconv.ParseInt(dataRate, 10, 64)
	if err != nil {
		log.Fatal("failed to parse data rate", err)

	}
	if s < 0 || s > 13 {
		log.Fatal("expected data rate should be between 0 and 13, actual:", s)
	}

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

	if band := os.Getenv("BAND"); band != "" {
		resp, err = lora.SetBand(band)
		if err != nil {
			return nil, errors.Wrapf(err, "set lora band")
		}
		log.Printf("lora module band set resp:%v", resp)
	}

	// If the received data is empty should increase the dr settings.
	// https://docs.exploratory.engineering/lora/dr_sf/
	// https://www.compel.ru/item-pdf/7008b5e14cfb8d82cebabdb784d57018/pn/rak~rak811.pdf
	config := "adr:off" + "&dr:" + dataRate + "&pwr_level:0" + "&dev_eui:" + devEUI + "&app_key:" + appKey + "&app_eui:0000010000000000" + "&nwks_key:00000000000000000000000000000000"
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
	now := time.Now()
	for {
		if debug {
			log.Print("sending join request attempt:", attempt)
		}
		resp, err = lora.JoinOTAA()
		if err != nil || attempt > 25 {
			log.Println("Reseting the module due to a join request err:", err, "or too many attempts:", attempt)
			lora.Close()
			return newLoraConnection(debug)
		}

		if resp == rak811.STATUS_JOINED_SUCCESS {
			log.Println("lora module joined, total join request duration:", time.Since(now))
			break
		}
		if debug {
			log.Print("unexpected gateway registration resp:", resp, " attempt:", attempt)
		}
		attempt++
	}
	return lora, nil
}
