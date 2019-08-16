TODO:
 - FIX POSGRESQL connection
 - "cloud" docker Exit code: "125" when starting the gpssender container.
 - when the loraserver is down the gps sender doesn't register that the data wasn't sent!!!
 - disable logging when in production to save SD card life. https://blog.hypriot.com/post/cloud-init-cloud-on-hypriot-x64/
        redirect docker to syslog and enable RAM disk of 100mb with logrotate. enough to see the latest logs. https://mcuoneclipse.com/2019/04/01/log2ram-extending-sd-card-lifetime-for-raspberry-pi-lorawan-gateway/
 - How to provide lora gateway keys before starting the gps sender - edit the file on ths SD - as an ENV variables in the ~/.bash_profile 
 - When shippied to denver how to update the image and redeploy the container? balena? ssh access? make image with dd and use etcher to burn to SD card
 - add CI for multi arch images.
 - update the same partol with new gps coordinates
 - document how setup the Loraserver.
 - Expose prometheus metrics for alerts?




