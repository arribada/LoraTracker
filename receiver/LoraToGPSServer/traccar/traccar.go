package traccar

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/brocaar/lorawan"
)

// NewHandler creates a new alert type handler.
func NewHandler() *Handler {
	a := &Handler{
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
	return a
}

// Handler is the alert type handler struct.
type Handler struct {
	httpClient *http.Client
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/traccar" {
		httpError(w, "unimplemented path:"+r.URL.Path, http.StatusNotImplemented)
		return
	}
	c, err := ioutil.ReadAll(r.Body)
	if err != nil {
		httpError(w, "reading request body err:"+err.Error(), http.StatusBadRequest)
		return
	}

	if os.Getenv("DEBUG") != "" {
		log.Printf("incoming request body:%v RemoteAddr:%v headers:%+v \n", string(c), r.RemoteAddr, r.Header)
	}

	data := &DataUpPayload{}
	err = json.Unmarshal(c, data)
	if err != nil {
		httpError(w, "unmarshaling request body err:"+err.Error(), http.StatusBadRequest)
		return
	}

	if data.FPort != 1 && data.FPort != 12 {
		if os.Getenv("DEBUG") != "" {
			log.Printf("skipping non gps data, fport:%+v", data.FPort)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	lat, ok := data.Object["lat"]
	latF := lat.(float64)
	if !ok || latF == 0 {
		if os.Getenv("DEBUG") != "" {
			log.Printf("skipping data with missing gps lat, body:%+v", data)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	lon, ok := data.Object["lon"]
	lonF := lon.(float64)
	if !ok || lonF == 0 {
		if os.Getenv("DEBUG") != "" {
			log.Printf("skipping data with missing gps lon, body:%+v", data)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	var snr float64
	var rssi int
	if len(data.RXInfo) == 0 {
		if os.Getenv("DEBUG") != "" {
			log.Println("received lora data doesn't include gateway meta data")
		}
	} else {
		snr = data.RXInfo[0].LoRaSNR
		rssi = data.RXInfo[0].RSSI
	}

	var batF float64
	if bat, ok := data.Object["battery"]; ok {
		batF = bat.(float64)
	}

	if os.Getenv("DEBUG") != "" {
		log.Printf("incoming request unmarshaled body:%+v", data)
	}

	server, ok := r.Header["Traccarserver"]
	if !ok || len(server) != 1 {
		httpError(w, "missing or incorrect traccarServer header", http.StatusBadRequest)
		return
	}

	_, err = url.ParseRequestURI(server[0])
	if err != nil {
		httpError(w, "invalid traccarServer url format expected: http://serverNameOrIP", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequest("GET", server[0], nil)
	if err != nil {
		httpError(w, "creating a new request:"+err.Error(), http.StatusInternalServerError)
		return
	}

	q := req.URL.Query()
	q.Add("id", data.DevEUI.String())
	q.Add("lat", fmt.Sprintf("%g", latF))
	q.Add("lon", fmt.Sprintf("%g", lonF))
	q.Add("battery", fmt.Sprintf("%g", batF))
	q.Add("snr", fmt.Sprintf("%g", snr))
	q.Add("rssi", strconv.Itoa(rssi))
	q.Add("battery", fmt.Sprintf("%g", batF))
	req.URL.RawQuery = q.Encode()

	res, err := s.httpClient.Do(req)
	if err != nil {
		httpError(w, "sending the  request err:"+err.Error(), http.StatusBadRequest)
		return
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		httpError(w, "unexpected response status code:"+strconv.Itoa(res.StatusCode)+"request:"+req.URL.RawQuery, http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
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

func httpError(w http.ResponseWriter, err string, code int) {
	_, fn, line, _ := runtime.Caller(1)
	log.Printf("[error] %s:%d %v", fn, line, err)
	http.Error(w, err, code)
}
