module github.com/arribada/LoraTracker/sender/GPSSender

go 1.12

require (
	github.com/adrianmo/go-nmea v1.1.1-0.20191002192055-6384a696ae32
	github.com/arribada/LoraTracker/sender/GPSSender/pkg/rak811 v0.0.0-00010101000000-000000000000
	github.com/brocaar/lorawan v0.0.0-20191115102621-6095d473cf60 // indirect
	github.com/davecheney/gpio v0.0.0-20160912024957-a6de66e7e470 // indirect
	github.com/kr/pretty v0.1.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.2.1 // indirect
	github.com/stretchr/testify v1.4.0 // indirect
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07
	github.com/twpayne/go-geom v1.0.5 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect

)

replace github.com/arribada/LoraTracker/sender/GPSSender/pkg/rak811 => ./pkg/rak811

// replace github.com/arribada/LoraTracker/receiver/LoraToGPSServer/smartConnect => ../../receiver/LoraToGPSServer/smartConnect
