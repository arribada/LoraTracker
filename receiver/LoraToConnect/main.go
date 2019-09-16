package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/arribada/SMARTLoraTracker/receiver/LoraToConnect/alerts"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

	alertsHandler := alerts.NewHandler()

	log.Println("starting server at port:", *receivePort)
	if os.Getenv("DEBUG") != "" {
		log.Println("displaying debug logs")
	}
	http.Handle("/", alertsHandler)
	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":"+*receivePort, nil))
}
