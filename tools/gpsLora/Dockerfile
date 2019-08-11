FROM --platform=$BUILDPLATFORM golang:alpine AS build
COPY ./ /tmp/builder/
WORKDIR /tmp/builder/
RUN CGO_ENABLED=0 go build  -o main .

FROM alpine
COPY --from=build main ./
CMD ./main