
![Blueprints](blueprints.svg)


TODO:
 - test adding a new device 
 - make a video how to add new devices.
 - remove the workaround for the dots in the env variables and remove from loraserver images and EnvToFile.
 - create an orb for setting up a working buildx env
 - update the same partol with new gps coordinates
 - at the end unexpose all ports for services that are not needed.


# Setup Pager duty account for the alerting(optional)
 - Sign up for an account.
 - Add a phone number under the profile Notification Rules.
 - Create a service.
 - Choose the integration as API V2.
 - copy the `Integration Key` add it as an env vairable with the balena setup.


# Initial Setup on Balena cloud

Create an application for the sender and the receiver: `LoraGpsSender`, `SMARTLoraReciever`.

## SMARTLoraReciever Setup

- Fleet configuration
```
RESIN_HOST_CONFIG_enable_uart
RESIN_HOST_CONFIG_dtparam "i2c_arm=on","spi=on","audio=on"
RESIN_HOST_CONFIG_dtoverlay pi3-disable-bt
RESIN_HOST_CONFIG_core_freq 250 // Seems that uart is more stable with this.
RESIN_HOST_CONFIG_gpu_mem 16mb
```

 - Env vars
```
APPLICATION__SERVER_EXTERNAL__API_JWT__SECRET=....
POSTGRES_USER=postgres
POSTGRES_PASSWORD=...
ROUTING_KEY=... # The "Integration Key" from Pager Duty for sending  alerts with the alert manager.
```



- Add a device and follow the UI steps.

- Install the [balena cli](https://github.com/balena-io/balena-cli) and apply the application compose file.

```
cd ./receiver
balena push SMARTLoraReciever
```
- Service Variables for the `appserver` service.
> replace the `...` with the value from the POSTGRES_PASSWORD env variable.

```
POSTGRESQL_DSN=postgres;//loraserver_as;...@postgresql/loraserver_as?sslmode=disable
```
- Service Variables for the `loraserver` service.
```
POSTGRESQL_DSN=postgres;//loraserver_ns;...@postgresql/loraserver_ns?sslmode=disable
```

### Access the applications:
Loraserver: http://deviceIPorDomain:8080<br/>
Login: admin admin

SMART connect: https://deviceIPorDomain:8443/server<br/>
Login: smart smart

### Setup loraserver

- Network-servers/Add
```
name: gpsTracker
server: loraserver:8000
```
- Service-profiles/Create
```
name: gpsTracker
server: gpsTracker
Add gateway meta-data: selected
```
- Device-profiles/Create
```
name: gpsTracker
server: gpsTracker
LoRaWAN MAC version: 1.0.3
LoRaWAN Regional Parameters revision: A
Join (OTAA / ABP): Device supports OTAA
```
- Gateways/Create
```
name: gpsTracker
description: gpsTracker
id:... #look for the gateway_ID in the compose file
server: gpsTracker
location: #drag the pin to the current gateway location. This determens when the gps tracker is outside a parimeter and when to send alerts.
```
- Applications/Create
```
name: gpsTracker
description: gpsTracker
profile: gpsTracker
codec:none
```
- Applications/gpsTracker/Devices/Create
```
name: gpsSender
description: gpsSender
EUI: generate random
profile: gpsTracker
```
Applications/gpsTracker/Devices/<br/>
tab: KEYS
```
Application key: generate random
```
- Applications/gpsTracker/Integrations/Create
```
kind: HTTP
headers:
    SMARTserver: https://smart-connect:8443
    SMARTcarea: get it from SMART connect
    SMARTuser: smart
    SMARTpass: smart
Uplink data URL: http://lora-connect:8070
```
## LoraGpsSender setup
 - Env vars
```
APP_KEY= // the one set in loraserver
DEV_EUI= // the one set in loraserver
```
 - Fleet configuration
```
RESIN_HOST_CONFIG_enable_uart
RESIN_HOST_CONFIG_dtoverlay pi3-miniuart-bt
```
- Now add a device and follow the UI steps.

- Apply the application compose file.

```
cd ./sender
balena push ApplicationName
```

## Adding an additional device

From the balena UI just select `Add a new device` and follow the on screen instructions.

- Select the latest OS
- Production image
