package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device"
	"github.com/arribada/LoraTracker/receiver/LoraToGPSServer/smartConnect"
	"github.com/arribada/LoraTracker/receiver/LoraToGPSServer/traccar"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile | log.Lmsgprefix)
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

	metrics := device.NewMetrics()
	smartConnectHandler := smartConnect.NewHandler(metrics)
	traccarHandler := traccar.NewHandler(metrics)

	log.Println("starting server at port:", *receivePort)
	if os.Getenv("DEBUG") == "1" {
		log.Println("with debug logs")
	}

	// Keep handlers separate so that if one server returns an error
	// it doesn't affect updates to the others.
	http.Handle("/smartConnect", smartConnectHandler)
	http.Handle("/traccar", traccarHandler)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+*receivePort, nil))
}
