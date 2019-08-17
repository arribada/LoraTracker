module github.com/arribada/loraTracker/tools/GPSSender

go 1.12

require (
	github.com/adrianmo/go-nmea v1.1.1-0.20190809134752-fb3e95815d06
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4 // indirect
	github.com/calvernaz/rak811 v0.0.0-20190816115655-12c9277ae11c
	github.com/kr/pretty v0.1.0 // indirect
	github.com/pkg/errors v0.8.1
	github.com/stretchr/testify v1.4.0 // indirect
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07
	golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)

// replace github.com/adrianmo/go-nmea => ../../../../adrianmo/go-nmea

replace github.com/calvernaz/rak811 => github.com/krasi-georgiev/rak811 v0.0.0-20190817120852-5e9aeb779eb7
