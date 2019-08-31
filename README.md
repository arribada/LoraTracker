
![Blueprints](blueprints.svg)


TODO:
 - generate consisten GW ID from the mac - now it changes on every restart.
 - create an orb for setting up a working buildx env

 - circle ci auto build for the loraserver images -WAIT FRO REPLY FORM UPSTREAM.
 - move the packet forwarder in a separate repo and auto build the images
 - 
 - better approach for not embeding the config files? this makes the images not costumazible.
 - update the same partol with new gps coordinates

 - Expose prometheus metrics for alerts?


Add env variables

RECEIVER
GW_ID=0242acfffe110006 // Can be any 16 characters long id. Used to match a given packet forwarder id and display the statistics.
POSTGRES_PASSWORD=postgres
POSTGRES_USER=postgres
Device configs
RESIN_HOST_CONFIG_core_freq 250
RESIN_HOST_CONFIG_dtoverlay pi3-disable-bt


SENDER
Env vars
app_key= // the one set in the lora server
dev_eui= // the one set in the lora server
Device configs
RESIN_HOST_CONFIG_enable_uart
RESIN_HOST_CONFIG_dtoverlay pi3-miniuart-bt



Setup the lora gateway

get the GATEWAYID http://loraserverIP:8090

Download and flash balena os to the flash card
for the rpi zero with wifi
download the production image.

 




