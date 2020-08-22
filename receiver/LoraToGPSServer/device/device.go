package device

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Data struct {
	Bat      float64
	Lat      float64
	Lon      float64
	Snr      float64
	Rssi     int
	Payload  *DataUpPayload
	Metadata map[string]string
	Type     string
	ID       string
	Valid    bool
}

func Parse(r *http.Request, metrics *Metrics) (*Data, error) {
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading request body")
	}

	if os.Getenv("DEBUG") == "1" {
		log.Printf("incoming request body:%v RemoteAddr:%v headers:%+v \n", string(c), r.RemoteAddr, r.Header)
	}

	data := &DataUpPayload{}
	err = json.Unmarshal(c, data)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshaling request body ")
	}

	devType, ok := data.Tags["type"]
	if !ok {
		return nil, fmt.Errorf("request payload doesn't include device type tags:%+v", data.Tags)
	}

	var dataParsed *Data

	switch devType {
	case "rpi":
		dataParsed, err = Rpi(string(data.Data))
	case "irnas":
		dataParsed, err = Irnas(data)
	default:
		return nil, fmt.Errorf("unsuported device type:%v", devType)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "parsing device data type:%v", devType)
	}

	dataParsed.Payload = data
	dataParsed.Type = devType
	dataParsed.ID = GenID(data)

	metrics.UpdateSignals(dataParsed)

	if len(data.RXInfo) == 0 {
		if os.Getenv("DEBUG") == "1" {
			log.Println("received lora data doesn't include gateway meta data")
		}
	} else {
		for i, g := range data.RXInfo {
			// Record only the signal from the nearest gateway.
			// Only the strongest signal.
			if i == 0 || g.LoRaSNR > dataParsed.Snr {
				dataParsed.Snr = g.LoRaSNR
			}
			if i == 0 || g.RSSI > dataParsed.Rssi {
				dataParsed.Rssi = g.RSSI
			}
		}
	}

	return dataParsed, err

}

func Rpi(data string) (*Data, error) {
	coordinates := strings.Split(string(data), ",")
	if len(coordinates) < 2 {
		return nil, fmt.Errorf("parsing the cordinates string:%v", data)
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(coordinates[0]), 64)
	if err != nil {
		return nil, errors.Wrap(err, "parsing the latitude string")

	}
	if lat < -90 || lat > 90 {
		return nil, errors.New("latitude outside acceptable values")
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(coordinates[1]), 64)
	if err != nil {
		return nil, errors.Wrap(err, "parsing the longitude string")

	}
	if lon < -180 || lon > 180 {
		return nil, errors.New("longitude outside acceptable values")
	}

	d := &Data{
		Lat:      lat,
		Lon:      lon,
		Metadata: map[string]string{},
		Valid:    true,
	}

	singlePoints := len(coordinates) == 3 && coordinates[2] == "s"
	if singlePoints {
		d.Metadata["s"] = "true"
	}

	return d, nil
}

func Irnas(data *DataUpPayload) (*Data, error) {
	dataParsed := &Data{}

	// Non GPS data.
	if data.FPort != 1 && data.FPort != 12 {
		if os.Getenv("DEBUG") == "1" {
			log.Printf("skipping non gps data, fport:%+v", data.FPort)
		}
		return dataParsed, nil
	}

	lat, ok := data.Object["lat"]
	if !ok {
		return nil, errors.New("data object doesn't contain lat")
	}
	lon, ok := data.Object["lon"]
	if !ok {
		return nil, errors.New("data object doesn't contain lon")
	}

	// When resent is more than 1 it means NO new gps coordinates are available and
	// the latest ones were resent so can be ignored.
	if val, ok := data.Object["gps_resend"]; !ok || val.(int) == 1 {
		dataParsed.Valid = true
	}

	dataParsed.Lat = lat.(float64)
	dataParsed.Lon = lon.(float64)
	if dataParsed.Lat != 0.0 && dataParsed.Lon != 0.0 {
		dataParsed.Valid = true
	}

	if bat, ok := data.Object["battery"]; ok {
		dataParsed.Bat = bat.(float64)
	}

	return dataParsed, nil
}

// DataUpPayload represents a data-up payload.
type DataUpPayload struct {
	ApplicationID   int64                  `json:"applicationID,string"`
	ApplicationName string                 `json:"applicationName"`
	DeviceName      string                 `json:"deviceName"`
	DevEUI          lorawan.EUI64          `json:"devEUI"`
	RXInfo          []RXInfo               `json:"rxInfo,omitempty"`
	TXInfo          TXInfo                 `json:"txInfo"`
	ADR             bool                   `json:"adr"`
	FCnt            uint32                 `json:"fCnt"`
	FPort           uint8                  `json:"fPort"`
	Data            []byte                 `json:"data"`
	Object          map[string]interface{} `json:"object,omitempty"`
	Tags            map[string]string      `json:"tags,omitempty"`
	Variables       map[string]string      `json:"-"`
}

// RXInfo contains the RX information.
type RXInfo struct {
	GatewayID lorawan.EUI64 `json:"gatewayID"`
	Name      string        `json:"name"`
	Time      *time.Time    `json:"time,omitempty"`
	RSSI      int           `json:"rssi"`
	LoRaSNR   float64       `json:"loRaSNR"`
	Location  *Location     `json:"location"`
}

// TXInfo contains the TX information.
type TXInfo struct {
	Frequency int `json:"frequency"`
	DR        int `json:"dr"`
}

// Location details.
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
}

func NewMetrics() *Metrics {
	return &Metrics{
		DistanceMeters: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "distance_meters",
				Help: "Distance in meters between the received gps coordinates and the gaetway location.",
			},
			[]string{"gateway_id", "dev_id"},
		),
		LastUpdate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "last_update_seconds",
				Help: "The time in seconds since the last update.",
			},
			[]string{"dev_id"},
		),
		Rssi: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "rssi",
				Help: "rssi of the received data.",
			},
			[]string{"gateway_id", "dev_id"},
		),
		Snr: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "snr",
				Help: "snr of the received data.",
			},
			[]string{"gateway_id", "dev_id"},
		),
	}
}

func (s *Metrics) UpdateSignals(data *Data) {
	if len(data.Payload.RXInfo) == 0 {
		if os.Getenv("DEBUG") == "1" {
			log.Println("received lora data doesn't include gateway meta data")
		}
	} else {
		// Distance from each gateway that received this data.
		for _, gwMeta := range data.Payload.RXInfo {
			s.DistanceMeters.With(prometheus.Labels{"gateway_id": gwMeta.GatewayID.String(), "dev_id": data.ID}).Set(distance(data.Lat, data.Lon, gwMeta.Location.Latitude, gwMeta.Location.Longitude, "K") * 1000)
			s.Rssi.With(prometheus.Labels{"gateway_id": gwMeta.GatewayID.String(), "dev_id": data.ID}).Set(float64(gwMeta.RSSI))
			s.Snr.With(prometheus.Labels{"gateway_id": gwMeta.GatewayID.String(), "dev_id": data.ID}).Set(float64(gwMeta.LoRaSNR))
			s.LastUpdate.With(prometheus.Labels{"dev_id": data.ID}).Set(0)
		}
	}
}

type Metrics struct {
	DistanceMeters *prometheus.GaugeVec
	LastUpdate     *prometheus.GaugeVec
	Rssi           *prometheus.GaugeVec
	Snr            *prometheus.GaugeVec
}

func distance(lat1 float64, lng1 float64, lat2 float64, lng2 float64, unit ...string) float64 {
	const PI float64 = 3.141592653589793

	radlat1 := float64(PI * lat1 / 180)
	radlat2 := float64(PI * lat2 / 180)

	theta := float64(lng1 - lng2)
	radtheta := float64(PI * theta / 180)

	dist := math.Sin(radlat1)*math.Sin(radlat2) + math.Cos(radlat1)*math.Cos(radlat2)*math.Cos(radtheta)

	if dist > 1 {
		dist = 1
	}

	dist = math.Acos(dist)
	dist = dist * 180 / PI
	dist = dist * 60 * 1.1515

	if len(unit) > 0 {
		if unit[0] == "K" {
			dist = dist * 1.609344
		} else if unit[0] == "N" {
			dist = dist * 0.8684
		}
	}

	return dist
}

func GenID(data *DataUpPayload) string {
	return data.DeviceName + "-" + data.DevEUI.String()
}
