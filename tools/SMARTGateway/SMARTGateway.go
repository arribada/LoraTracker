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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/pkg/errors"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	app := kingpin.New(filepath.Base(os.Args[0]), "A tool that listens for lora packets and send them to a remote SMART connect server")
	app.HelpFlag.Short('h')

	SMARTserver := app.Flag("SMARTserver", "server api url").
		Required().
		Short('s').
		String()
	SMARTuser := app.Flag("SMARTuser", "login username").
		Default("smart").
		Short('u').
		String()
	SMARTpass := app.Flag("SMARTpass", "login pass").
		Default("smart").
		Short('w').
		String()
	SMARTca := app.Flag("SMARTcarea", "conservation area to upload the file to").
		Required().
		Short('c').
		String()
	receivePort := app.Flag("listenPort", "http port to listen to for incomming lora packets").
		Default("8080").
		Short('p').
		String()

	if _, err := app.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		app.Usage(os.Args[1:])
		os.Exit(2)
	}

	handler := newHandler(*SMARTserver, *SMARTuser, *SMARTpass, *SMARTca)

	log.Println("starting server at port:", *receivePort)
	http.Handle("/", handler)
	log.Fatal(http.ListenAndServe(":"+*receivePort, nil))

}

func newHandler(server, user, pass, ca string) *Handler {
	return &Handler{
		server: server,
		user:   user,
		pass:   pass,
		ca:     ca,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		alertID: defaultAlertID,
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
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println("reading request body err:", err)
	}

	data := &DataUpPayload{}
	err = json.Unmarshal(c, data)
	if err != nil {
		log.Println("unmarshaling request body err:", err)
	}

	if err := s.createAlert(w, r, data); err != nil {
		log.Println("creating an alert err:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	log.Println("new alert created", "application name", data.ApplicationName, "sensor name", data.DeviceName)

	if err := s.createPatrolUpload(w, r, data); err != nil {
		log.Println("creating an upload err:", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
	log.Println("new upload created", "application name:", data.ApplicationName, "device name:", data.DeviceName)

}

func (s *Handler) createAlert(w http.ResponseWriter, r *http.Request, data *DataUpPayload) error {
	exists, err := s.alertIDExists()
	if err != nil {
		return fmt.Errorf("checking that the alert ID exists err:%v", err)
	}
	if !exists {
		log.Println("creating a missing alert type UUID:", s.alertID)
		s.alertID, err = s.createAlertTypeRequest(data)
		if err != nil {
			return fmt.Errorf("checking that the alert ID exists err:%v", err)
		}
		log.Println("new alert type UUID:", s.alertID)
	}
	if err := s.createAlertRequest(data); err != nil {
		return fmt.Errorf("sending alert create request to the SMART connect server err:%v", err)
	}

	return nil
}

func (s *Handler) createPatrolUpload(w http.ResponseWriter, r *http.Request, data *DataUpPayload) error {
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
        <ns2:days date="2019-06-13" startTime="00:00:00" endTime="23:59:59" restMinutes="0.0"/>
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
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response status code:%v", res.StatusCode)
	}

	uploadURL, err := res.Location()
	if err != nil {
		return fmt.Errorf("getting response location err:%v", err)
	}
	res.Body.Close()

	// Make the actual request to upload the file.
	{
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("upload_file", filepath.Base(fileName))
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

func (s *Handler) alertIDExists() (bool, error) {
	url := s.server + "/server/api/connectalert/alertTypes"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Println(err)
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

func (s *Handler) createAlertTypeRequest(data *DataUpPayload) (string, error) {
	url := s.server + "/server/api/connectalert/alertTypes/" + data.DevEUI.String()

	var jsonStr = []byte(`
	{
		"label":"` + data.DevEUI.String() + `",
		"color":"5AFF54",
		"opacity":".80",
		"markerIcon":"cloud",
		"markerColor":"blue",
		"spin":"false",
		"customIcon":"99"
	  }`)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Println(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(s.user, s.pass)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Println(err)
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

func (s *Handler) createAlertRequest(data *DataUpPayload) error {
	url := s.server + "/server/api/connectalert/" + data.DevEUI.String()

	var jsonStr = []byte(`
	{
		"type":"FeatureCollection",
		"features":[
			{
				"type":"Feature",
				"geometry":	{
					"type":"Point",
					"coordinates":["26.440139600000002","40.474172599999996"]
				},
				"properties":{
					"deviceId":"` + data.DevEUI.String() + `",
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

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response status code:%v", res.StatusCode)
	}
	return nil
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
	UUID string `json:"uuid"`
}
