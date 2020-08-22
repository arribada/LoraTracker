package smartConnect

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkt"
)

// NewHandler creates a new alert type handler.
func NewHandler(metrics *device.Metrics) *Handler {
	a := &Handler{
		metrics: metrics,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		allDevIDs: make(map[string]string),
		careasBuf: make(map[string]struct{}),
	}
	a.incLastUpdateTime()
	return a
}

// Handler is the alert type handler struct.
type Handler struct {
	server,
	user,
	pass,
	ca string
	httpClient *http.Client
	// allDevIDs is used to reduce the SMART connect API calls
	// when checking if an alert type exists.
	allDevIDs map[string]string
	// careasBuf is used to reduce the SMART connect API calls
	// when checking is CA area exists.
	careasBuf map[string]struct{}
	mtx       sync.Mutex
	metrics   *device.Metrics
}

// incLastUpdateTime increases the update time to detect when a device has lost a signal.
func (s *Handler) incLastUpdateTime() {
	go func() {
		t := time.NewTicker(time.Second).C
		for {
			select {
			case <-t:
				s.mtx.Lock()
				for devID := range s.allDevIDs {
					s.metrics.LastUpdate.With(prometheus.Labels{"dev_id": devID}).Inc()
				}
				s.mtx.Unlock()
			}
		}
	}()
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, err := device.Parse(r, s.metrics)
	if err != nil {
		httpError(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.SetPrefix("devName:" + data.Payload.DeviceName)

	if !data.Valid {
		if os.Getenv("DEBUG") == "1" {
			log.Printf("skipping data with invalid gps coords, body:%+v", data)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if !data.Valid && os.Getenv("DEBUG") == "1" {
		log.Printf("skipping data with invalid gps coords, body:%+v", data)
		w.WriteHeader(http.StatusOK)
		return
	}

	server, ok := r.Header["Smartserver"]
	if !ok || len(server) != 1 {
		httpError(w, "missing or incorrect SmartServer header", http.StatusBadRequest)
		return
	}

	_, err = url.ParseRequestURI(server[0])
	if err != nil {
		httpError(w, "invalid SmartServer url format expected: https://serverNameOrIP", http.StatusBadRequest)
		return
	}

	user, ok := r.Header["Smartuser"]
	if !ok || len(user) != 1 {
		httpError(w, "missing or incorrect SmartUser header", http.StatusBadRequest)
		return
	}
	pass, ok := r.Header["Smartpass"]
	if !ok || len(pass) != 1 {
		httpError(w, "missing or incorrect SmartPass header", http.StatusBadRequest)
		return
	}
	carea, ok := r.Header["Smartcarea"]
	if !ok || len(carea) != 1 {
		httpError(w, "missing or incorrect SmartCarea header", http.StatusBadRequest)
		return
	}

	s.server = server[0]
	s.user = user[0]
	s.pass = pass[0]
	s.ca = carea[0]

	if _, ok := s.careasBuf[carea[0]]; !ok {
		exists, err := s.careaExists(carea[0])
		if err != nil {
			httpError(w, "checking if a  conservation area exists, err:"+err.Error(), http.StatusBadRequest)
			return
		}
		if !exists {
			httpError(w, "conservation area doesn't exist:"+carea[0], http.StatusNotFound)
			return
		}

		// Reset the buffer if too big.
		if len(s.careasBuf) > 100 {
			s.careasBuf = make(map[string]struct{})
		}
		s.careasBuf[carea[0]] = struct{}{}
	}

	if err := s.createAlert(w, r, data); err != nil {
		httpError(w, "creating an alert err:"+err.Error(), http.StatusBadRequest)
		return
	}

	fileContent, ok := r.Header["Smartdesktopfile"]
	if !ok || len(fileContent) != 1 {
		if os.Getenv("DEBUG") == "1" {
			log.Printf("Smartdesktopfile header is empty so NOT creating an upload for SMART desktop")
		}
	} else {
		if err := s.createPatrolUpload(w, r, []byte(fileContent[0])); err != nil {
			httpError(w, "creating an upload err:"+err.Error(), http.StatusBadRequest)
		}
		log.Println("new upload created")
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Handler) createAlert(w http.ResponseWriter, r *http.Request, data *device.Data) error {
	var err error

	// When the device id is present in all alerts map this guarantees that
	// the alert type exists so no need to create it.
	s.mtx.Lock()
	alertID, ok := s.allDevIDs[data.ID]
	s.mtx.Unlock()
	if !ok {
		alertID, err = s.alertID(data.ID)
		if err != nil {

			return fmt.Errorf("getting the alert id by the device ID:%v err:%v", data.ID, err)
		}
		// AlertID with this devID doesn't exists so need to create it.
		if alertID == "" {
			log.Println("alert type with the given device label doesn't exist so creating a new one dev id:", data.ID)
			alertID, err = s.createAlertType(data.ID)
			if err != nil {
				return fmt.Errorf("creating a new alertType for devID:%v err:%v", data.ID, err)
			}
		}
		s.mtx.Lock()
		s.allDevIDs[data.ID] = alertID
		s.mtx.Unlock()
	}

	url := s.server + "/server/api/connectalert/"
	// Use the same alert identifier when want to have a continious line
	// or use the current time as unique identifier when want to display each alert as an  individual point.
	if _, single := data.Metadata["s"]; single {
		url += data.ID
	} else {
		url += strconv.Itoa(int(time.Now().UnixNano()))
	}

	var jsonStr = []byte(`
	{
		"type":"FeatureCollection",
		"features":[
			{
				"type":"Feature",
				"geometry":	{
					"type":"Point",
					"coordinates":["` + strconv.FormatFloat(data.Lon, 'f', -1, 64) + `","` + strconv.FormatFloat(data.Lat, 'f', -1, 64) + `"]
				},
				"properties":{
					"deviceId":"` + data.ID + `",
					"id":"0",
					"latitude":0,
					"longitude":0,
					"altitude":0,
					"accuracy":0,
					"caUuid":"` + s.ca + `",
					"level":"1",
					"description":"",
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
	log.Println("alert created , request:", req.URL.RawQuery)

	response := &SMARTAlertType{}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if os.Getenv("DEBUG") == "1" {
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

func (s *Handler) createPatrolUpload(w http.ResponseWriter, r *http.Request, fileContent []byte) error {
	_, _ = wkt.Marshal(geom.NewPoint(geom.XY).MustSetCoords(geom.Coord{1, 2}))
	// if err != nil {
	// 	return fmt.Errorf("marshal geo location err:%v", err)
	// }
	fileName := "patrol.xml"
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

	if os.Getenv("DEBUG") == "1" {
		log.Println("CA area check response body:", string(body))
	}

	return strings.Contains(string(body), `"uuid":"`+s.ca+`"`), nil
}

func (s *Handler) createCarea(data *device.Data) error {
	url := s.server + "/server/api/conservationarea?cauuid=" + s.ca + "&name=" + data.Payload.ApplicationName
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
		"spin":"false"
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
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("reading the response body err:", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("invalid status code:%v body:%v request:%v", resp.Status, body, jsonStr)
	}

	response := &SMARTAlertType{}
	err = json.Unmarshal(body, response)
	if err != nil {
		return "", fmt.Errorf("unmarshal the response reply err:%v , response body:%v", err, body)
	}
	return response.UUID, nil
}

// SMARTAlertType details.
type SMARTAlertType struct {
	UUID     string `json:"uuid"`
	TypeUUID string `json:"typeUuid"`
	Label    string `json:"label"`
}

func httpError(w http.ResponseWriter, error string, code int) {
	log.Println(error)
	http.Error(w, error, code)
}
