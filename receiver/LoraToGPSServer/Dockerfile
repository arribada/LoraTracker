FROM --platform=$BUILDPLATFORM golang:alpine AS build
RUN apk add --no-cache git ca-certificates
COPY ./ /tmp/builder/
WORKDIR /tmp/builder/


# Install TARGETPLATFORM parser to translate its value to GOOS, GOARCH, and GOARM
COPY --from=tonistiigi/xx:golang / /

# Bring TARGETPLATFORM to the build scope
ARG TARGETPLATFORM

ENV GO111MODULE=on
ENV CGO_ENABLED=0
RUN go build  -o main .

FROM alpine
COPY --from=build /tmp/builder/main ./
ENTRYPOINT [ "./main" ]