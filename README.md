
![Blueprints](blueprints.svg)


# Initial Setup on Balena cloud

Create an application for the sender and the receiver: `LoraGpsSender`, `LoraGpsReciever`.
If you use standalone GPS tags for sending don't need to create an app for the sender.

## LoraGpsReciever Setup
> Skip when using Lorix one for receiving.

- When using the Lora hat for recieving add these env vars in the fleet configuration.
```
RESIN_HOST_CONFIG_enable_uart
RESIN_HOST_CONFIG_dtparam "i2c_arm=on","spi=on","audio=on"
RESIN_HOST_CONFIG_dtoverlay pi3-disable-bt
RESIN_HOST_CONFIG_core_freq 250 // Seems that uart is more stable with this.
RESIN_HOST_CONFIG_gpu_mem 16mb
```

 - Add fleet env vars
 
 For the Chirpstack server.
```
APPLICATION_SERVER__EXTERNAL_API__JWT_SECRET=.... # Choose one
POSTGRES_USER=postgres
POSTGRES_PASSWORD=... # Choose one
# The chirpstack network server band settings. 
# The default is EU_863_870 so if used in europe can skip this var.
#For all possible options see https://www.chirpstack.io/network-server
NETWORK_SERVER__BAND__NAME =
```
 
- Env vars for the Rpi Lora hat.
> Skip if using Lorix one.
```
# The semtech gateway setting. See https://github.com/arribada/packet-forwarder
CONCENTRATOR_CONFIG= 
```


- Add a device and follow the UI steps.

- Install the [balena cli](https://github.com/balena-io/balena-cli) and apply the application compose file.

```
cd ./receiver
balena push FleetName # The selected fleet name when creating the fleet.
```
- At fleet level add service variables
> replace the `...` with the value from the POSTGRES_PASSWORD env variable.

for the `chirpstack-appserver` service.
```
POSTGRESQL__DSN=postgres://chirpstack_as:...@chirpstack-postgresql/chirpstack_as?sslmode=disable
```
for the `chirpstack-networkserver` service.
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

> If the redis server doesn't start after lorix restart
> ```
> /usr/bin/redis-check-aof --fix /var/lib/redis/appendonly.aof <<< 'yes'
> ```

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
    - For Rpi sender
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
description: gpsTracker
# for Rpi sender - look for the gateway_ID in the sender's compose file or in the corresponding env variable  if overridden by one.
# for Lorix one - http://deviceIPorDomain:8080/lora/forwarder or in the config file:` /etc/lora-packet-forwarder/global_conf.json` 
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

  - Rpi sender
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
    name: irnas
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

### Setup Chirpstack to send the data to other systems(optional).

#### <b>SMART connect</b>

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

#### <b>Traccar</b>
- Applications/gpsSender/Integrations/http
```
headers:
    traccarServer: http://traccar:5055
# Or the IP if not on the same machine as the packet forwarder.
Uplink data URL: http://lora-gps-server:8070/traccar
```
> multiple uplink urls are separated by coma:<br/>
> http://lora-gps-server:8070/smartConnect, http://lora-gps-server:8070/traccar

## LoraGpsSender setup
> skip when not using the Rpi sender.
 - Env vars
```
APP_KEY= // the one set in Chirpstack app server
DEV_EUI= // the one set in Chirpstack app server
BAND= // by default is is set to EU868 , other possible values are: AS923, EU868, AU915, US915, IN865, KR920
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
balena push LoraGpsSender
```

## Traccar setup
Add each tracker as device with its corresponding Device EUI(no empty spaces between the pairs. All lower case).

## Smart Desktop setup

If you want to upload data into SMART desktop it needs to be connected to SMART connect and also set the content of the data to be uploaded as a header in the chirpstack HTTP integration setup.
 - Install the Smart connect plugins.
 - Setup the connection to SMART connect. It requires HTTPS and for this can use the default certificate in https://github.com/arribada/SMARTConnect
 - Create an example Patrol and export it. This will be used as a template.
 - Take the content of the Patrol file and set it as chirpstack HTTP integration header.