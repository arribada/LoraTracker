
![Blueprints](blueprints.svg)


# Initial Setup on Balena cloud

> Follow the steps in the exact order as the initial boostrap relies on given env variables to be present.

Create an application for the sender and the receiver: `LoraGpsSender`, `LoraGpsReciever`.
If you use standalone GPS tags for sending don't need to create an app for the sender.

## LoraGpsReciever Setup

### At fleet level add env vars

 - For the Rpi Lora hat (Skip when using Lorix one for receiving)

```
 # The semtech gateway setting. See https://github.com/arribada/packet-forwarder
CONCENTRATOR_CONFIG=
```

 - For the Chirpstack server.
```
APPLICATION_SERVER__EXTERNAL_API__JWT_SECRET=.... # Choose one
POSTGRES_USER=postgres
POSTGRES_PASSWORD=... # Choose one
# The chirpstack network server band settings. 
# The default is EU_863_870 so if used in europe can skip this var.
#For all possible options see https://www.chirpstack.io/network-server
NETWORK_SERVER__BAND__NAME =
```

### Fleet configuration (Skip when using Lorix one for receiving)
```
RESIN_HOST_CONFIG_enable_uart
RESIN_HOST_CONFIG_dtparam "i2c_arm=on","spi=on","audio=on"
RESIN_HOST_CONFIG_dtoverlay pi3-disable-bt
RESIN_HOST_CONFIG_core_freq 250 // Seems that uart is more stable with this.
RESIN_HOST_CONFIG_gpu_mem 16mb
```


### Add a device and follow the UI steps.

- Install the [balena cli](https://github.com/balena-io/balena-cli) and apply the application compose file.

```
cd ./receiver
balena push FleetNameReciever # The selected fleet name when creating the fleet.
```
### At fleet level add service variables
> replace the `...` with the value from the POSTGRES_PASSWORD env variable.

 - for the `chirpstack-appserver` service.
```
POSTGRESQL__DSN=postgres://chirpstack_as:...@chirpstack-postgresql/chirpstack_as?sslmode=disable
```
 - for the `chirpstack-networkserver` service.
```
POSTGRESQL__DSN=postgres://chirpstack_ns:...@chirpstack-postgresql/chirpstack_ns?sslmode=disable
```

### Access the applications:
Enable the option PUBLIC DEVICE URL and click the link next to the option.
Chirpstack App Server: http://url:8080<br/>
Login: admin admin

SMART connect: https://url:8443/server<br/>
Login: smart smart

TracCar: https://url<br/>
Login: admin    admin

### Setup chirpstack app server

- Network-servers/Add
```
name: local
server: chirpstack-networkserver:8000 # For lorix `localhost:8000`
```
- Service-profiles/Create
```
name: gpsTracker
server: gpsTracker
Add gateway meta-data: selected
```
- Device-profiles/Create
    - For Rpi sender (skip when not using the Rpi sender)
        ```
        name: rpi
        server: local
        LoRaWAN MAC version: 1.0.3
        LoRaWAN Regional Parameters revision: A
        Join (OTAA / ABP): Device supports OTAA
        ```
    - For Irnas sender
        ```
        name: irnas
        server: local
        LoRaWAN MAC version: 1.0.3
        LoRaWAN Regional Parameters revision: A
        Codec: Custom JavaScript codec functions
            For the decode field use the file decoder.js from the configs folder
            For the encode field use the file encoder.js from the configs folder
        ```
- Gateways/Create
```
name: main
description: anything
# for Rpi sender - look for the gateway_ID in the sender's compose file or in the corresponding env variable  if overridden by one.
# for Lorix one - http://lorixOneIP/lora/forwarder or in the config file:` /etc/lora-packet-forwarder/global_conf.json`
id:...
server: main
location: #drag the pin to the current gateway location. This determines when a gps tag is outside a parimeter and when to send Prometheus alerts.
```
- Applications/Create
```
name: gpsTracker
description: gpsTracker
profile: gpsTracker
codec:none
```
- Applications/gpsTracker/Devices/Create

  - Rpi sender (skip when not using the Rpi sender)
    ```
    name: rpi
    description: rpi
    EUI: generate random # write it down as it will be used when setting up the Rpi sender
    profile: rpi
    Disable frame-counter validation: selected
    Tags: 
        type    rpi
    
    # The fields belod show only after creating the device.
    Tab KEYS:
        Application key: generate random # write it down as it will be used when setting up the sender

    ```
  - Irnas sender
    ```
    name: (the tag ID)
    description: irnas
    EUI: # Take it from https://console.thethingsnetwork.org/
    profile: irnas
    Disable frame-counter validation: selected
    Tags: 
        type    irnas
    
    # The fields belod show only after creating the device.
    # Take all these from https://console.thethingsnetwork.org/
    Device address:
    Network session key:
    Application session key:
    ```

### Setup Lorix to connect to Chirpstack

 - Login to lorix using the WEB gui with `admin` and `lorix4u`
 - Navigate to Lora -> Forwarder -> Select `Chirpstack Gateway Bridge`
 - Edit the `Bridge configuration` and under the `mqtt` section add the IP address of the Rpi like `server="tcp://192.168.1.188:1883"`. For now Lorix OS doesn't support mDNS names so need to set a static address for the IP so that it doesn't change between restarts.
 - Save and click the `Start` button. The logs should show no errors which means it is connected to chirpstack

### Setup Chirpstack to send the data to other systems(optional).

#### SMART connect

- Applications/gpsTracker/Integrations/Create
```
kind: HTTP
headers:
    SMARTserver: https://smart-connect:8443
    SMARTcarea: get it from SMART connect
    SMARTuser: smart
    SMARTpass: smart
    SMARTDesktopFile: # Optional header if you want to create an upload to Smart Desktop. See the section for Smart Desktop setup.
# Or the IP if not on the same machine as the packet forwarder.
Uplink data URL: http://lora-gps-server:8070/smartConnect
```

#### Traccar
- Applications/gpsSender/Integrations/http
```
Payload marshaler: JSON legacy
headers:
    traccarServer: http://traccar:5055
# Or the IP if not on the same machine as the packet forwarder.
Uplink data URL: http://lora-gps-server:8070/traccar
```
> multiple uplink urls are separated by coma:<br/>
> http://lora-gps-server:8070/smartConnect, http://lora-gps-server:8070/traccar

## LoraGpsSender setup
> skip when not using the Rpi sender.

### At fleet level add env vars
```
APP_KEY= // the one set in Chirpstack app server
DEV_EUI= // the one set in Chirpstack app server
BAND= // by default is is set to EU868 , other possible values are: AS923, EU868, AU915, US915, IN865, KR920
```
### Fleet configuration
```
RESIN_HOST_CONFIG_enable_uart
RESIN_HOST_CONFIG_dtoverlay pi3-miniuart-bt
```
- Now add a device and follow the UI steps.

- Apply the application compose file.

```
cd ./sender
balena push FleetNameSender
```

## Traccar setup
Add each tracker as device with its corresponding Device EUI(no empty spaces between the pairs. All lower case).

## Smart Desktop setup

If you want to upload data into SMART desktop it needs to be connected to SMART connect and also set the content of the data to be uploaded as a header in the chirpstack HTTP integration setup.
 - Install the Smart connect plugins.
 - Setup the connection to SMART connect. It requires HTTPS and for this can use the default certificate in https://github.com/arribada/SMARTConnect
 - Create an example Patrol and export it. This will be used as a template.
 - Take the content of the Patrol file and set it as chirpstack HTTP integration header.

### Notes

If traccar fails to start with - `Waiting for changelog lock`
 - stop the container and run the following commands on the device.
 - `balena run -it --rm -v 1853980_traccar-database:/opt/traccar/data --entrypoint=/bin/bash traccar/traccar:4.14-ubuntu`
 - `java -cp lib/h2*.jar org.h2.tools.Shell -url "jdbc:h2:/opt/traccar/data/database" -driver org.h2.Driver -user sa`
 - `SELECT * FROM PUBLIC.DATABASECHANGELOGLOCK;`
 - `update PUBLIC.DATABASECHANGELOGLOCK set locked=0 WHERE ID=1;`
