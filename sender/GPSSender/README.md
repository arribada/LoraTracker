The Dockerfile is only for arm32v6 Rpi Zero.

Can be pushed to Rpi running `balena push appName` from within the directory.

Env Vars

SEND_FAKE_GPS - when the gps cannot locate signal send some fake gps to test the lora connection.
DEBUG=1 - enable debug logging.
HDOP - set a minimum HDOP accuracy level. Usually anything below 1.50 is good
APP_KEY - required lora server app key
DEV_EUI - required lora server dev key