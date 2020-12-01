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
	"sync"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Data struct {
	Lat     float64
	Lon     float64
	Snr     float64
	Rssi    int
	Payload *DataUpPayload
	Attr    map[string]string
	Type    string
	ID      string
	Valid   bool
	Speed   float64
	Time    int64 // The gps fix time in epoch timestamp.
	Motion  bool
}

func NewManager() *Manager {
	mn := &Manager{
		metrics:   NewMetrics(),
		allDevIDs: make(map[string]*Data),
	}
	mn.incLastUpdateTime()
	return mn
}

type Manager struct {
	lastFCnt uint32
	metrics  *Metrics

	mtx sync.Mutex

	// allDevIDs holds the last data update for all devices.
	allDevIDs map[string]*Data
}

func (self *Manager) Parse(r *http.Request) (*Data, error) {
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

	// Update the metrics only for non duplicate requests.
	// A duplicate request happens because the lora server is set to send
	// the same request for each backend server - traccar, smart connect etc.
	if self.lastFCnt != data.FCnt {
		if err := self.update(dataParsed); err != nil {
			return nil, err
		}
	}
	self.lastFCnt = data.FCnt

	if dataParsed.Motion {
		dataParsed.Speed = self.Speed(dataParsed.ID)
	}

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

func (self *Manager) update(data *Data) error {
	self.mtx.Lock()
	defer self.mtx.Unlock()

	if len(data.Payload.RXInfo) == 0 {
		if os.Getenv("DEBUG") == "1" {
			log.Println("received lora data doesn't include gateway meta data")
		}
	} else {
		// Distance from each gateway that received this data.
		for _, gwMeta := range data.Payload.RXInfo {
			if data.Valid {
				dist, err := Distance(data.Lat, data.Lon, gwMeta.Location.Latitude, gwMeta.Location.Longitude, "K")
				if err != nil {
					return err
				}
				self.metrics.distanceMeters.With(prometheus.Labels{"gateway_id": gwMeta.GatewayID.String(), "dev_id": data.ID}).Set(dist * 1000)
			}
			self.metrics.rssi.With(prometheus.Labels{"gateway_id": gwMeta.GatewayID.String(), "dev_id": data.ID}).Set(float64(gwMeta.RSSI))
			self.metrics.snr.With(prometheus.Labels{"gateway_id": gwMeta.GatewayID.String(), "dev_id": data.ID}).Set(float64(gwMeta.LoRaSNR))
			self.metrics.lastUpdate.With(prometheus.Labels{"dev_id": data.ID}).Set(0)
		}
	}

	if lastUpdate, ok := self.allDevIDs[data.ID]; ok && lastUpdate.Valid && data.Valid {
		speed, err := Speed(lastUpdate, data)
		if err != nil {
			return err
		}
		data.Speed = speed
	}
	self.allDevIDs[data.ID] = data

	return nil
}

func (s *Manager) Speed(devID string) float64 {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.allDevIDs[devID].Speed
}

// incLastUpdateTime increases the update time to detect when a device has lost a signal.
func (s *Manager) incLastUpdateTime() {
	go func() {
		t := time.NewTicker(time.Second).C
		for range t {
			s.mtx.Lock()
			for devID := range s.allDevIDs {
				s.metrics.lastUpdate.With(prometheus.Labels{"dev_id": devID}).Inc()
			}
			s.mtx.Unlock()
		}
	}()
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
		Lat:    lat,
		Lon:    lon,
		Attr:   map[string]string{},
		Valid:  true,
		Time:   time.Now().Unix(),
		Motion: true,
	}

	singlePoints := len(coordinates) == 3 && coordinates[2] == "s"
	if singlePoints {
		d.Attr["s"] = "true"
	}

	return d, nil
}

func Irnas(data *DataUpPayload) (*Data, error) {
	dataParsed := &Data{
		Valid: true,
		Attr:  map[string]string{},
	}

	// Non GPS data.
	if data.FPort != 1 && data.FPort != 12 {
		dataParsed.Valid = false
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
	if val, ok := data.Object["gps_resend"]; ok && val.(float64) != 1.0 {
		dataParsed.Valid = false
	}

	dataParsed.Lat = lat.(float64)
	dataParsed.Lon = lon.(float64)
	if dataParsed.Lat == 0.0 || dataParsed.Lon == 0.0 {
		dataParsed.Valid = false
	}
	if val, ok := data.Object["gps_time"]; ok { // From system updates.
		dataParsed.Time = int64(val.(float64))
	}
	if val, ok := data.Object["time"]; ok { // From periodic or motion triggered updates.
		dataParsed.Time = int64(val.(float64))
	}

	if val, ok := data.Object["battery"]; ok {
		dataParsed.Attr["battery"] = fmt.Sprintf("%v", val.(float64))
	}
	if val, ok := data.Object["motion"]; ok && int64(val.(float64)) > 0 {
		dataParsed.Motion = true
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
	m := &Metrics{
		distanceMeters: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "distance_meters",
				Help: "Distance in meters between the received gps coordinates and the gaetway location.",
			},
			[]string{"gateway_id", "dev_id"},
		),
		lastUpdate: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "last_update_seconds",
				Help: "The time in seconds since the last update.",
			},
			[]string{"dev_id"},
		),
		rssi: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "rssi",
				Help: "rssi of the received data.",
			},
			[]string{"gateway_id", "dev_id"},
		),
		snr: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "snr",
				Help: "snr of the received data.",
			},
			[]string{"gateway_id", "dev_id"},
		),
	}
	return m
}

type Metrics struct {
	distanceMeters *prometheus.GaugeVec
	lastUpdate     *prometheus.GaugeVec
	rssi           *prometheus.GaugeVec
	snr            *prometheus.GaugeVec
}

func Distance(lat1 float64, lon1 float64, lat2 float64, lon2 float64, unit ...string) (float64, error) {
	const PI float64 = 3.141592653589793

	radlat1 := float64(PI * lat1 / 180)
	radlat2 := float64(PI * lat2 / 180)

	theta := float64(lon1 - lon2)
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
			return dist * 1.609344, nil
		} else if unit[0] == "N" {
			return dist * 0.8684, nil
		}
	}

	return 0, fmt.Errorf("invalid metric unit:%v", unit)
}

// Speed calculates speed in knots from the distance between 2 gps points.
func Speed(point1 *Data, point2 *Data) (float64, error) {
	km, err := Distance(point1.Lat, point1.Lon, point2.Lat, point2.Lon, "K")
	if err != nil {
		return 0, err
	}
	if km == 0.0 {
		return 0.0, nil
	}

	// Use Abs in case of an out of order updates.
	// For the distance calculation it doesn't matter,
	// but for the time diff we don't want negative numbers.
	timeDiff := math.Abs(float64(point2.Time - point1.Time))

	hr := timeDiff / 3600.0
	kmh := km / hr
	knots := kmh / 1.8520001412492

	return knots, nil
}

func GenID(data *DataUpPayload) string {
	return data.DeviceName + "-" + data.DevEUI.String()
}
