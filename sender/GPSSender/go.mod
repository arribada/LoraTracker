module github.com/arribada/loraTracker/tools/GPSSender

go 1.12

require (
	github.com/adrianmo/go-nmea v1.1.1-0.20190909160214-4effbc117043
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4 // indirect
	github.com/arribada/SMARTLoraTracker/sender/GPSSender/pkg/rak811 v0.0.0-00010101000000-000000000000 // indirect
	github.com/arribada/rak811 v0.0.0-20190826193639-e58c41454f63
	github.com/kr/pretty v0.1.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/stretchr/objx v0.2.0 // indirect
	github.com/stretchr/testify v1.4.0 // indirect
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07
	golang.org/x/sys v0.0.0-20190826190057-c7b8b68b1456 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
)

replace github.com/arribada/SMARTLoraTracker/sender/GPSSender/pkg/rak811 => ./pkg/rak811
