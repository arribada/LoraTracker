module github.com/arribada/loraTracker/tools/gpsLora

go 1.12

require (
	github.com/adrianmo/go-nmea v1.1.1-0.20190603151144-9861e243767d
	github.com/calvernaz/rak811 v0.0.0-20190804084735-96a5f835ea87
	github.com/pkg/errors v0.8.1
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07
)

replace github.com/adrianmo/go-nmea => ../../../../adrianmo/go-nmea
