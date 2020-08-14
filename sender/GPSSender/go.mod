module github.com/arribada/LoraTracker/sender/GPSSender

go 1.12

require (
	github.com/adrianmo/go-nmea v1.1.1-0.20191002192055-6384a696ae32
	github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device v0.0.0-20200814180814-8cfe5cee5bb2
	github.com/arribada/LoraTracker/sender/GPSSender/pkg/rak811 v0.0.0-00010101000000-000000000000
	github.com/davecheney/gpio v0.0.0-20160912024957-a6de66e7e470 // indirect
	github.com/pkg/errors v0.9.1
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07

)

replace github.com/arribada/LoraTracker/sender/GPSSender/pkg/rak811 => ./pkg/rak811
replace github.com/arribada/LoraTracker/receiver/LoraToGPSServer/device => ../../receiver/LoraToGPSServer/device
