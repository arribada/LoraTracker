package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
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

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkt"
	"gopkg.in/alecthomas/kingpin.v2"
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
	log.Fatal(http.ListenAndServe(":"+*receivePort, nil))

}

func newHandler() *Handler {
	return &Handler{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		alertID: defaultAlertID,
		// careasBuf is a buffer to reduce the API calls.
		careasBuf: make(map[string]struct{}),
	}
}

const defaultAlertID = "b9bb1bd0-52ec-47a2-8908-0b599244fb69"

type Handler struct {
	server,
	user,
	pass,
	ca,
	alertID string
	httpClient *http.Client
	careasBuf  map[string]struct{}
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("reading request body err:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if os.Getenv("DEBUG") != "" {
		log.Println("incoming request body:", string(c))
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
	log.Println("alert created", "application", data.ApplicationName, "sensor", genDevID(data))

	// if err := s.createPatrolUpload(w, r, data); err != nil {
	// 	log.Println("creating an upload err:", err)
	// 	http.Error(w, err.Error(), http.StatusBadRequest)
	// }
	// log.Println("new upload created", "application:", data.ApplicationName, "device:", genDevID(data))
	w.WriteHeader(http.StatusOK)
}

func (s *Handler) createAlert(w http.ResponseWriter, r *http.Request, data *DataUpPayload) error {
	url := s.server + "/server/api/connectalert/" + genDevID(data)

	coordinates := strings.Split(string(data.Data), ",")
	if len(coordinates) < 2 {
		return errors.New("parsing the cordinates string")

	}

	latitude, err := strconv.ParseFloat(coordinates[0], 64)
	if err != nil {
		return errors.Errorf("parsing the latitude string err:%v", err)

	}
	if latitude < -90 || latitude > 90 {
		return errors.New("latitude outside acceptable values")
	}
	longitude, err := strconv.ParseFloat(coordinates[0], 64)
	if err != nil {
		return errors.Errorf("parsing the longitude string err:%v", err)

	}
	if longitude < -180 || longitude > 180 {
		return errors.New("longitude outside acceptable values")
	}

	var jsonStr = []byte(`
	{
		"type":"FeatureCollection",
		"features":[
			{
				"type":"Feature",
				"geometry":	{
					"type":"Point",
					"coordinates":["` + coordinates[1] + `","` + coordinates[0] + `"]
				},
				"properties":{
					"deviceId":"` + genDevID(data) + `",
					"id":"0",
					"latitude":0,
					"longitude":0,
					"altitude":0,
					"accuracy":0,
					"caUuid":"` + s.ca + `",
					"level":"1",
					"description":"` + string(data.Data) + `",
					"typeUuid":"` + s.alertID + `",
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
	// Empty typeUuid means that the alert type doesn't exists to need to create it.
	if response.TypeUUID == "00000000-0000-0000-0000-000000000000" {
		log.Println("creating a missing alert type UUID:", s.alertID)
		s.alertID, err = s.createAlertType(data)
		if err != nil {
			return fmt.Errorf("checking that the alert ID exists err:%v", err)
		}
		log.Println("new alert type UUID:", s.alertID)
		return s.createAlert(w, r, data)
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

func (s *Handler) alertIDExists() (bool, error) {
	url := s.server + "/server/api/connectalert/alertTypes"
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

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("invalid status code response: %v", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(body), `"uuid":"`+s.alertID+`"`), nil
}

func (s *Handler) createAlertType(data *DataUpPayload) (string, error) {
	url := s.server + "/server/api/connectalert/alertTypes/" + genDevID(data)

	var jsonStr = []byte(`
	{
		"label":"` + genDevID(data) + `",
		"color":"FF0526",
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
}
