module github.com/arribada/SMARTLoraTracker/receiver/LoraToConnect

go 1.12

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4 // indirect
	github.com/arribada/SMARTLoraTracker/receiver/LoraToConnect/alerts v0.0.0-00010101000000-000000000000
	github.com/brocaar/lorawan v0.0.0-20190725071148-7d77cf375455 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v1.1.0
	github.com/twpayne/go-geom v1.0.6-0.20190712172859-6e5079ee5888 // indirect
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
)

replace github.com/arribada/SMARTLoraTracker/receiver/LoraToConnect/alerts => ./alerts
