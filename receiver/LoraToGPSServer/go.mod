module github.com/arribada/SMARTLoraTracker/receiver/LoraToGPSServer

go 1.12

require (
	github.com/arribada/SMARTLoraTracker/receiver/LoraToGPSServer/smartConnect v0.0.0-00010101000000-000000000000
	github.com/brocaar/lorawan v0.0.0-20191115102621-6095d473cf60
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.6.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

replace github.com/arribada/SMARTLoraTracker/receiver/LoraToGPSServer/smartConnect => ./smartConnect

replace github.com/arribada/SMARTLoraTracker/receiver/LoraToGPSServer/traccar => ./traccar
