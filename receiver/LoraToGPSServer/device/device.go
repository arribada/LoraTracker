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
	Hdop    float64
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

func (self *Manager) Parse(r *http.Request) ([]*Data, error) {
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading request body")
	}

	if os.Getenv("DEBUG") == "1" {
		log.Printf("incoming request body:%v RemoteAddr:%v headers:%+v \n", string(c), r.RemoteAddr, r.Header)
	}
	fmt.Println(string(c))
	data := &DataUpPayload{}
	err = json.Unmarshal(c, data)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshaling request body")
	}

	devType, ok := data.Tags["type"]
	if !ok {
		return nil, fmt.Errorf("request payload doesn't include device type tags:%+v", data.Tags)
	}
	var _ *Data
	var points []*Data

	switch devType {
	case "rpi":
		points, err = Rpi(string(data.Data))
	case "antratek":
		points, err = Antratek(data)
	case "irnas":
		points, err = Irnas(data)
	case "Second":
		points, err = G62Parse(data)
	default:
		return nil, fmt.Errorf("unsuported device type:%v", devType)
	}
	if err != nil {
		return nil, errors.Wrapf(err, "parsing device data type:%v", devType)
	}

	for i, point := range points {
		point.Payload = data
		point.Type = devType
		point.ID = GenID(data)

		// Update the metrics only for non duplicate requests.
		// A duplicate request happens because the lora server is set to send
		// the same request for each backend server - traccar, smart connect etc.
		if self.lastFCnt != data.FCnt {
			if err := self.update(point); err != nil {
				return nil, err
			}
		}
		self.lastFCnt = data.FCnt

		if point.Motion {
			point.Speed = self.Speed(point.ID)
		}

		if len(data.RXInfo) == 0 {
			if os.Getenv("DEBUG") == "1" {
				log.Println("received lora data doesn't include gateway meta data")
			}
		} else {
			for i, g := range data.RXInfo {
				// Record only the signal from the nearest gateway.
				// Only the strongest signal.
				if i == 0 || g.LoRaSNR > point.Snr {
					point.Snr = g.LoRaSNR
				}
				if i == 0 || g.RSSI > point.Rssi {
					point.Rssi = g.RSSI
				}
			}
		}

		points[i] = point
	}

	return points, err

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
			if gwMeta.Location == nil {
				if os.Getenv("DEBUG") == "1" {
					log.Println("recieved meta gateway without location")
				}
				continue
			}
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

func Rpi(data string) ([]*Data, error) {
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

	return []*Data{d}, nil
}

type dataInterface map[string]interface{}

func Irnas(data *DataUpPayload) ([]*Data, error) {
	dataParsed := &Data{
		Valid: true,
		Attr:  map[string]string{},
	}

	// Non GPS data.
	if data.FPort != 1 && data.FPort != 12 && data.FPort != 11 {
		dataParsed.Valid = false
		if os.Getenv("DEBUG") == "1" {
			log.Printf("skipping non gps data, fport:%+v", data.FPort)
		}
		return []*Data{dataParsed}, nil
	}

	logRaw, ok := data.Object["locations"]
	if !ok {
		d, err := irnasParseSingle(data.Object)
		return []*Data{d}, err
	}

	logs := make([]dataInterface, 5)

	err := json.Unmarshal([]byte(logRaw.(string)), &logs)
	if err != nil {
		return nil, errors.Wrap(err, "parsing locations json")
	}

	var logsParsed []*Data

	for _, log := range logs {
		data, err := irnasParseSingle(log)
		if err != nil {
			return nil, errors.Wrap(err, "parsing single location")
		}
		logsParsed = append(logsParsed, data)
	}

	return logsParsed, nil
}

func irnasParseSingle(data dataInterface) (*Data, error) {
	dataParsed := &Data{
		Valid: true,
		Attr:  map[string]string{},
	}

	lat, ok := data["lat"]
	if !ok {
		lat, ok = data["latitude"]
		if !ok {
			return nil, errors.New("data doesn't contain lat")
		}
	}
	lon, ok := data["lon"]
	if !ok {
		lon, ok = data["longitude"]
		if !ok {
			return nil, errors.New("data object doesn't contain lon")
		}
	}

	// Port 12 status messages contain only lat/lon.
	hdop, ok := data["hdop"]
	if !ok {
		log.Printf("data object doesn't contain hdop so setting to 0")
		hdop = 0.0
	}
	dataParsed.Hdop = hdop.(float64)

	dataParsed.Lat = lat.(float64)
	dataParsed.Lon = lon.(float64)
	if dataParsed.Lat == 0.0 || dataParsed.Lon == 0.0 {
		dataParsed.Valid = false
	}
	if val, ok := data["gps_time"]; ok { // From system updates.
		dataParsed.Time = int64(val.(float64))
	}
	if val, ok := data["time"]; ok { // From periodic or motion triggered updates.
		dataParsed.Time = int64(val.(float64))
	}

	if val, ok := data["battery"]; ok {
		dataParsed.Attr["battery"] = fmt.Sprintf("%v", val.(float64))
	}
	if val, ok := data["motion"]; ok && int64(val.(float64)) > 0 {
		dataParsed.Motion = true
	}

	return dataParsed, nil
}




func G62Parse(data *DataUpPayload) ([]*Data, error) {

	dataParsed := &Data{
		Valid: true,
		Attr:  map[string]string{},
	}

	// Non GPS data.
	if data.FPort != 1 {
		dataParsed.Valid = false
		if os.Getenv("DEBUG") == "1" {
			log.Printf("skipping non gps data, fport:%+v", data.FPort)
		}
		return []*Data{dataParsed}, nil
	}

	lat, ok := data.Object["latitudeDeg"]
	if !ok {
		return nil, errors.New("data doesn't contain lat")
	}
	lon, ok := data.Object["longitudeDeg"]
	
	if !ok {
		return nil, errors.New("data object doesn't contain lon")
	}
	//data.DevEUI=data.DeviceInfo.DevEui

	dataParsed.Time = int64( data.Time.Unix())
	

	// Port 12 status messages contain only lat/lon.
	hdop, ok := data.Object["hdop"]
	if !ok {
		log.Printf("data object doesn't contain hdop so setting to 0")
		hdop = 0.0
	}
	dataParsed.Hdop = hdop.(float64)

	dataParsed.Lat = lat.(float64)
	dataParsed.Lon = lon.(float64)
	if dataParsed.Lat == 0.0 || dataParsed.Lon == 0.0 {
		dataParsed.Valid = false
	}
	/*if val, ok := data["gps_time"]; ok { // From system updates.
		dataParsed.Time = int64(val.(float64))
	}
	if val, ok := data["time"]; ok { // From periodic or motion triggered updates.
		dataParsed.Time = int64(val.(float64))
	}*/

	if val, ok := data.Object["speedKmph"]; ok {
		dataParsed.Attr["speedKmph"] = fmt.Sprintf("%v", val.(float64))
	}
	if val, ok := data.Object["tempC"]; ok && int64(val.(float64)) > 0 {
		dataParsed.Attr["temperature"] = fmt.Sprintf("%v", val.(float64))
	}

	return []*Data{dataParsed}, nil
}

func antratekParse(data *DataUpPayload) ([]*Data, error) {

	dataParsed := &Data{
		Valid: true,
		Attr:  map[string]string{},
	}

	// Non GPS data.
	if data.FPort != 136 {
		dataParsed.Valid = false
		if os.Getenv("DEBUG") == "1" {
			log.Printf("skipping non gps data, fport:%+v", data.FPort)
		}
		return []*Data{dataParsed}, nil
	}

	lat, ok := data.Object["positionLatitude"]
	if !ok {
		return nil, errors.New("data doesn't contain lat")
	}
	lon, ok := data.Object["positionLongitude"]
	
	if !ok {
		return nil, errors.New("data object doesn't contain lon")
	}
	//data.DevEUI=data.DeviceInfo.DevEui

	dataParsed.Time = int64( data.Time.Unix())
	

	// Port 12 status messages contain only lat/lon.
	hdop, ok := data.Object["hdop"]
	if !ok {
		log.Printf("data object doesn't contain hdop so setting to 0")
		hdop = 0.0
	}
	dataParsed.Hdop = hdop.(float64)

	dataParsed.Lat = lat.(float64)
	dataParsed.Lon = lon.(float64)
	if dataParsed.Lat == 0.0 || dataParsed.Lon == 0.0 {
		dataParsed.Valid = false
	}
	/*if val, ok := data["gps_time"]; ok { // From system updates.
		dataParsed.Time = int64(val.(float64))
	}
	if val, ok := data["time"]; ok { // From periodic or motion triggered updates.
		dataParsed.Time = int64(val.(float64))
	}*/

	if val, ok := data.Object["battery"]; ok {
		dataParsed.Attr["battery"] = fmt.Sprintf("%v", val.(float64))
	}
	if val, ok := data.Object["temperature"]; ok && int64(val.(float64)) > 0 {
		dataParsed.Attr["temperature"] = fmt.Sprintf("%v", val.(float64))
	}

	return []*Data{dataParsed}, nil
}

// DataUpPayload represents a data-up payload.
type DataUpPayload struct {
	Time       *time.Time             `json:"time"`
	TXInfo     TXInfo                 `json:"txInfo"`
	ADR        bool                   `json:"adr"`
	FCnt       uint32                 `json:"fCnt"`
	FPort      uint8                  `json:"fPort"`
	Data       []byte                 `json:"data"`
	Object     map[string]interface{} `json:"object,omitempty"`
	Variables  map[string]string      `json:"-"`
	DeviceInfo `json:"deviceInfo"`
}

// DataUpPayload represents a data-up payload.
type DataUpPayloadAntratek struct {
	DeduplicationId string `json:"deduplicationId"`

	DeviceInfo []DeviceInfo `json:"deviceInfo"`
}

type DeviceInfo struct {
	TenantId          string            `json:"tenantId"`
	TenantName        string            `json:"tenantName"`
	ApplicationName   string            `json:"applicationName"`
	ApplicationId     string            `json:"applicationId"`
	DeviceProfileId   string            `json:"deviceProfileId"`
	DeviceProfileName string            `json:"deviceProfileName"`
	DeviceName        string            `json:"deviceName"`
	DevEui            lorawan.EUI64     `json:"devEui"`
	RXInfo            []RXInfo          `json:"rxInfo,omitempty"`
	Tags              map[string]string `json:"tags"`
}

type Tags struct {
	Type string `json:"type"`
}

// RXInfo contains the RX information.
type RXInfo struct {
	GatewayID lorawan.EUI64 `json:"gatewayID"`
	Name      string        `json:"name"`

	RSSI     int       `json:"rssi"`
	LoRaSNR  float64   `json:"loRaSNR"`
	Location *Location `json:"location"`
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
	return data.DeviceName + "-" + data.DeviceInfo.DevEui.String()
}
