package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkt"
	"gopkg.in/alecthomas/kingpin.v2"
)

var distanceMeters = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "distance_meters",
		Help: "Distance in meters between the received gps coordinates and the gaetway location.",
	},
	[]string{"gateway_id", "dev_id"},
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	app := kingpin.New(filepath.Base(os.Args[0]), "A tool that listens for lora packets and send them to a remote SMART connect server")
	app.HelpFlag.Short('h')

	receivePort := app.Flag("listenPort", "http port to listen to for incomming lora packets").
		Default("8070").
		Short('p').
		String()

	if _, err := app.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		app.Usage(os.Args[1:])
		os.Exit(2)
	}

	handler := newHandler()

	log.Println("starting server at port:", *receivePort)
	if os.Getenv("DEBUG") != "" {
		log.Println("displaying debug logs")
	}
	http.Handle("/", handler)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+*receivePort, nil))

}

func newHandler() *Handler {
	return &Handler{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		alertIDBuf: make(map[string]string),
		careasBuf:  make(map[string]struct{}),
	}
}

type Handler struct {
	server,
	user,
	pass,
	ca string
	httpClient *http.Client
	// alertIDBuf is used to reduce the API calls.
	alertIDBuf map[string]string
	// careasBuf is used to reduce the API calls.
	careasBuf map[string]struct{}
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("reading request body err:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if os.Getenv("DEBUG") != "" {
		log.Println("incoming request body:", string(c), "RemoteAddr:", r.RemoteAddr)
	}

	data := &DataUpPayload{}
	err = json.Unmarshal(c, data)
	if err != nil {
		log.Println("unmarshaling request body err:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	server, ok := r.Header["Smartserver"]
	if !ok || len(server) != 1 {
		http.Error(w, "missing or incorrect Smartserver header", http.StatusBadRequest)
	}

	_, err = url.ParseRequestURI(server[0])
	if err != nil {
		http.Error(w, "invalid Smartserver url format expected: https://serverNameOrIP", http.StatusBadRequest)
	}

	user, ok := r.Header["Smartuser"]
	if !ok || len(user) != 1 {
		http.Error(w, "missing or incorrect Smartuser header", http.StatusBadRequest)
	}
	pass, ok := r.Header["Smartpass"]
	if !ok || len(pass) != 1 {
		http.Error(w, "missing or incorrect Smartpass header", http.StatusBadRequest)
	}
	carea, ok := r.Header["Smartcarea"]
	if !ok || len(carea) != 1 {
		http.Error(w, "missing or incorrect Smartcarea header", http.StatusBadRequest)
	}

	s.server = server[0]
	s.user = user[0]
	s.pass = pass[0]
	s.ca = carea[0]

	if _, ok := s.careasBuf[carea[0]]; !ok {
		exists, err := s.careaExists(carea[0])
		if err != nil {
			http.Error(w, "checking if a  conservation area exists, err:"+err.Error(), http.StatusBadRequest)
			return
		}
		if !exists {
			if os.Getenv("DEBUG") != "" {
				log.Println("CA area doesn't exist uuid:", s.ca)
			}
			http.Error(w, "conservation area doesn't exist", http.StatusNotFound)
			return
		}

		// Reset the buffer if too big.
		if len(s.careasBuf) > 100 {
			s.careasBuf = make(map[string]struct{})
		}
		s.careasBuf[carea[0]] = struct{}{}
	}

	if err := s.createAlert(w, r, data); err != nil {
		log.Println("creating an alert err:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Println("alert created for application:", data.ApplicationName, ",device id:", genDevID(data))

	// if err := s.createPatrolUpload(w, r, data); err != nil {
	// 	log.Println("creating an upload err:", err)
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// }
	// log.Println("new upload created", "application:", data.ApplicationName, "device:", genDevID(data))
	w.WriteHeader(http.StatusOK)
}

func (s *Handler) createAlert(w http.ResponseWriter, r *http.Request, data *DataUpPayload) error {
	var err error
	lat, long, err := parseCoordinates(string(data.Data))
	if err != nil {
		return err
	}

	devID := genDevID(data)

	if len(data.RXInfo) == 0 {
		log.Println("received lora data doesn't include gateway meta data")
	} else {
		// Distance from each gateway that received this data.
		for _, gwMeta := range data.RXInfo {
			distanceMeters.With(prometheus.Labels{"gateway_id": gwMeta.GatewayID.String(), "dev_id": devID}).Set(distance(lat, long, gwMeta.Location.Latitude, gwMeta.Location.Longitude, "K") * 1000)
		}
	}
	alertID, ok := s.alertIDBuf[devID]
	if !ok {
		alertID, err = s.alertID(devID)
		if err != nil {
			return fmt.Errorf("getting the alert id by the device devID err:%v", err)
		}
		// AlertID with this devID doesn't exists so need to create it.
		if alertID == "" {
			log.Println("alert type with the given device label doesn't exist so creating a new one dev id:", devID)
			alertID, err = s.createAlertType(devID)
			if err != nil {
				return fmt.Errorf("creating a new alertType for devID:%v err:%v", devID, err)
			}
		}

		// Reset the buffer if too big.
		if len(s.alertIDBuf) > 100 {
			s.alertIDBuf = make(map[string]string)
		}
		s.alertIDBuf[devID] = alertID
	}

	url := s.server + "/server/api/connectalert/" + genDevID(data)

	var jsonStr = []byte(`
	{
		"type":"FeatureCollection",
		"features":[
			{
				"type":"Feature",
				"geometry":	{
					"type":"Point",
					"coordinates":["` + strconv.FormatFloat(long, 'f', -1, 64) + `","` + strconv.FormatFloat(lat, 'f', -1, 64) + `"]
				},
				"properties":{
					"deviceId":"` + devID + `",
					"id":"0",
					"latitude":0,
					"longitude":0,
					"altitude":0,
					"accuracy":0,
					"caUuid":"` + s.ca + `",
					"level":"1",
					"description":"` + string(data.Data) + `",
					"typeUuid":"` + alertID + `",
					"sighting":{}
					}
			}
		]
	}`)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return fmt.Errorf("creating a request err:%v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	res, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending the request err:%v", err)
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected response status code:%v", res.StatusCode)
	}

	response := &SMARTAlertType{}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if os.Getenv("DEBUG") != "" {
		log.Printf("SMART connect reply status:%v, body:%v", res.StatusCode, string(body))
	}
	err = json.Unmarshal(body, response)
	if err != nil {
		return err
	}
	// Empty typeUuid means that the alert type doesn't exists.
	// This shouldn't happen.
	if response.TypeUUID == "00000000-0000-0000-0000-000000000000" {
		log.Println("creating an alert returned an empty  'TypeUUID'")
	}

	return nil
}

func (s *Handler) createPatrolUpload(w http.ResponseWriter, r *http.Request, data *DataUpPayload) error {
	geo, err := wkt.Marshal(geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{1, 2}))
	if err != nil {
		return fmt.Errorf("marshal geo location err:%v", err)
	}
	fileName := "patrol.xml"
	fileContent := []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ns2:patrol xmlns:ns2="http://www.smartconservationsoftware.org/xml/1.2/patrol" patrolType="GROUND" startDate="2019-06-13" endDate="2019-06-13" isArmed="false" id="SMART_000007">
    <ns2:objective>
        <ns2:description></ns2:description>
    </ns2:objective>
    <ns2:team languageCode="en" value="Community Team 1"/>
    <ns2:station languageCode="en" value="Fixed Patrol Post 1"/>
    <ns2:legs startDate="2019-06-13" endDate="2019-06-13" id="1">
        <ns2:transportType languageCode="en" value="Foot"/>
        <ns2:members givenName="David" familyName="Aliata" employeeId="195000012" isPilot="false" isLeader="true"/>
		<ns2:days date="2019-06-13" startTime="00:00:00" endTime="23:59:59" restMinutes="0.0">
			<ns2:track distance="0.05490675941109657" geom="` + geo + `"/>
		</ns2:days>
        <ns2:mandate languageCode="en" value="Reasearch and Monitoring"/>
    </ns2:legs>
	<ns2:comment></ns2:comment>
</ns2:patrol>`)

	requestJSON := []byte(`
	{
		"conservationArea":"` + s.ca + `",
		"type":"PATROL_XML",
		"name":"` + fileName + `"
	 }
	`)
	req, err := http.NewRequest("POST", s.server+"/server/api/dataqueue/items/", bytes.NewBuffer(requestJSON))
	if err != nil {
		return fmt.Errorf("creating an upload request err:%v", err)
	}
	req.SetBasicAuth(s.user, s.pass)
	req.Header.Add("X-Upload-Content-Length", strconv.Itoa(len(fileContent)))
	req.Header.Set("Content-Type", "application/json")
	res, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending file upload request err:%v", err)
	}
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("unexpected response status code:%v", res.StatusCode)
	}

	uploadURL, err := res.Location()
	if err != nil {
		return fmt.Errorf("getting response location err:%v", err)
	}
	res.Body.Close()

	// Make the actual file upload.
	{
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("upload_file", fileName)
		if err != nil {
			return fmt.Errorf("creating an upload form err:%v", err)
		}

		io.Copy(part, bytes.NewReader(fileContent))
		writer.Close()
		req, err := http.NewRequest("POST", uploadURL.String(), body)
		if err != nil {
			return fmt.Errorf("creating an upload request err:%v", err)
		}

		req.Header.Add("Content-Type", writer.FormDataContentType())
		req.SetBasicAuth(s.user, s.pass)
		res, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("sending the upload request err:%v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusAccepted {
			return fmt.Errorf("unexpected response status code:%v", res.StatusCode)
		}
	}
	return nil
}

func (s *Handler) careaExists(ca string) (bool, error) {
	url := s.server + "/server/api/conservationarea"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return false, fmt.Errorf("invalid status code response: %v", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if os.Getenv("DEBUG") != "" {
		log.Println("CA area check response body:", string(body))
	}

	return strings.Contains(string(body), `"uuid":"`+s.ca+`"`), nil
}

func (s *Handler) createCarea(data *DataUpPayload) error {
	url := s.server + "/server/api/conservationarea?cauuid=" + s.ca + "&name=" + data.ApplicationName
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("invalid status code response:%v", resp.Status)
	}

	return nil
}

func (s *Handler) alertID(devID string) (string, error) {
	url := s.server + "/server/api/connectalert/alertTypes"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("invalid status code response: %v", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	alertTypes := make([]SMARTAlertType, 0)
	err = json.Unmarshal(body, &alertTypes)
	if err != nil {
		return "", err
	}
	for _, alertType := range alertTypes {
		if alertType.Label == devID {
			return alertType.UUID, nil
		}
	}
	return "", nil
}

func (s *Handler) createAlertType(label string) (string, error) {
	url := s.server + "/server/api/connectalert/alertTypes/" + label

	var jsonStr = []byte(`
	{
		"label":"` + label + `",
		"color":"FF0000",
		"opacity":".80",
		"markerIcon":"car",
		"markerColor":"black",
		"spin":"false",
	  }`)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("invalid status code response:%v", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	response := &SMARTAlertType{}
	err = json.Unmarshal(body, response)
	if err != nil {
		return "", err
	}
	return response.UUID, nil
}

func genDevID(data *DataUpPayload) string {
	return data.DeviceName + "-" + data.DevEUI.String()
}
func parseCoordinates(raw string) (float64, float64, error) {
	coordinates := strings.Split(string(raw), ",")
	if len(coordinates) < 2 {
		return 0, 0, errors.New("parsing the cordinates string")

	}

	latitude, err := strconv.ParseFloat(coordinates[0], 64)
	if err != nil {
		return 0, 0, errors.Errorf("parsing the latitude string err:%v", err)

	}
	if latitude < -90 || latitude > 90 {
		return 0, 0, errors.New("latitude outside acceptable values")
	}
	longitude, err := strconv.ParseFloat(coordinates[1], 64)
	if err != nil {
		return 0, 0, errors.Errorf("parsing the longitude string err:%v", err)

	}
	if longitude < -180 || longitude > 180 {
		return 0, 0, errors.New("longitude outside acceptable values")
	}
	return latitude, longitude, nil
}

// DataUpPayload represents a data-up payload.
type DataUpPayload struct {
	ApplicationID   int64             `json:"applicationID,string"`
	ApplicationName string            `json:"applicationName"`
	DeviceName      string            `json:"deviceName"`
	DevEUI          lorawan.EUI64     `json:"devEUI"`
	RXInfo          []RXInfo          `json:"rxInfo,omitempty"`
	TXInfo          TXInfo            `json:"txInfo"`
	ADR             bool              `json:"adr"`
	FCnt            uint32            `json:"fCnt"`
	FPort           uint8             `json:"fPort"`
	Data            []byte            `json:"data"`
	Object          interface{}       `json:"object,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
	Variables       map[string]string `json:"-"`
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

type SMARTAlertType struct {
	UUID     string `json:"uuid"`
	TypeUUID string `json:"typeUuid"`
	Label    string `json:"label"`
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
