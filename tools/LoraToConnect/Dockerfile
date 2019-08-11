FROM --platform=$BUILDPLATFORM golang:alpine AS build
RUN apk add --no-cache git
COPY ./ /tmp/builder/
WORKDIR /tmp/builder/
RUN CGO_ENABLED=0 go build  -o main .

FROM alpine
COPY --from=build main ./
CMD ./main