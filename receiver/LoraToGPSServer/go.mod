module github.com/arribada/LoraTracker/receiver/LoraToGPSServer

go 1.12

require (
	github.com/brocaar/lorawan v0.0.0-20210809075358-95fc1667572e
	// github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device v0.0.0-00010101000000-000000000000
	// github.com/arribada/LoraTracker/receiver/LoraToGPSServer/smartConnect v0.0.0-00010101000000-000000000000
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/twpayne/go-geom v1.4.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

// replace github.com/arribada/LoraTracker/receiver/LoraToGPSServer/smartConnect => ./smartConnect

// replace github.com/arribada/LoraTracker/receiver/LoraToGPSServer/traccar => ./traccar

// replace github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device => ./device
