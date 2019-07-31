package main

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/adrianmo/go-nmea"
	"github.com/calvernaz/rak811"
	"github.com/tarm/serial"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	cfg := &serial.Config{
		Name:        "/dev/ttyAMA0",
		ReadTimeout: 20000 * time.Millisecond,
	}
	lora, err := rak811.New(cfg)
	if err != nil {
		log.Fatal(err, "failed to create rak811 instance")
	}

	if err := lora.HardReset(); err != nil {
		log.Fatal("failed to reset module err:", err)
	}

	resp, err := lora.SetMode(0)
	if err != nil {
		log.Fatal("failed to set mode err:", err)
	}
	fmt.Println("set mode", resp)

	resp, err = lora.SetConfig("nwks_key:01020304050607080910111213141516&dev_eui:3038383664388108&app_key:01020304050607080910111213141516&app_eui:0000010000000000")
	if err != nil {
		log.Fatal("failed to set config err:", err)
	}
	fmt.Println("config", resp)

	resp, err = lora.JoinOTAA()
	if err != nil {
		log.Fatal("failed to join:", err)
	}
	fmt.Println("join", resp)

	for index := 0; index < 10; index++ {

		gpsD, err := readGPS()
		if err != nil {
			fmt.Println("error reading gps", resp)
		}

		data := hex.EncodeToString([]byte(gpsD.Time.String()))
		resp, err = lora.Send("0,1," + data)
		if err != nil {
			log.Fatal("failed to send data:", err)
		}
		fmt.Println("send", resp)

		time.Sleep(10 * time.Second)

	}

}

func readGPS() (*nmea.RMC, error) {
	c := &serial.Config{Name: "/dev/ttyUSB0", Baud: 9600, ReadTimeout: 10000 * time.Second}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}

	s.Write([]byte("b'$'"))
	// Turn on just minimum info (RMC only, location):
	s.Write([]byte("b'PMTK314,0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0'"))
	s.Write([]byte("b'*'"))
	s.Write([]byte("b'29'"))
	s.Write([]byte("b'\r\n'"))
	s.Write([]byte("b'$'"))
	// Set update rate to once a second (1hz) which is what you typically want.
	s.Write([]byte("b'PMTK220,1000'"))
	s.Write([]byte("b'*'"))
	s.Write([]byte("b'1F'"))
	s.Write([]byte("b'\r\n'"))

	reader := bufio.NewReader(s.Reader())

	defer s.Flush()
	defer s.Close()

	for {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	fmt.Println(strings.TrimSpace(line))

	ss, err := nmea.Parse(strings.TrimSpace(line))
	if err != nil {
		log.Fatal(err)
	}

	if ss.DataType() == nmea.TypeRMC {
		data := ss.(nmea.RMC)

		fmt.Println(data)


		// return &data, nil
	}
}

return nil, fmt.Errorf("invalid data")
}
