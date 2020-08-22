module github.com/arribada/LoraTracker/receiver/LoraToGPSServer/smartConnect

go 1.13

require (
	github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device v0.0.0-00010101000000-000000000000
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.7.1
	github.com/twpayne/go-geom v1.1.0
)


replace github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device => ./device
