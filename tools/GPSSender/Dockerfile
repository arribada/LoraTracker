FROM balenalib/raspberry-pi-alpine-golang:1.12 AS build
RUN apk add --no-cache git ca-certificates
COPY ./ /tmp/builder/
WORKDIR /tmp/builder/

ENV GO111MODULES=on
ENV CGO_ENABLED=0
RUN go build  -o main .

FROM balenalib/raspberry-pi-alpine
COPY --from=build /tmp/builder/main ./
ENV ENV UDEV=1
ENTRYPOINT [ "./main" ]