The Dockerfile is only for arm32v6 Rpi Zero.

Can be pushed to Rpi with `balena push appName` from within the directory.

Env Vars

SEND_FAKE_GPS - when the gps cannot locate signal send some fake gps to test the lora connection. If the content of the env variable contains coordinates in the format 42.6948...,23.360 they are used instead.
DEBUG=1 - enable debug logging.
HDOP - set a minimum HDOP accuracy level. Usually anything below 1.50 is good
APP_KEY - required lora server app key
DEV_EUI - required lora server dev key
DEV_ADDR  - required when using ABP
BAND - set the frequency band. One of:"EU868", "US915", "AU915", "KR920", "AS923"
DATA_RATE - set the lora data rate - https://docs.exploratory.engineering/lora/dr_sf
SINGLE_POINTS=1 - send updates as single points or continious line.
SEND_FREQ=..  - how often to send updates. Integer in seconds.