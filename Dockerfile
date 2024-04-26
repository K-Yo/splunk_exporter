# syntax=docker/dockerfile:1

FROM golang:1.21 AS build

ARG SRC=.

WORKDIR /app

COPY ${SRC}/go.mod ${SRC}/go.mod ./
RUN go mod download

COPY ${SRC} ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /splunk_exporter

FROM scratch

COPY --from=build /splunk_exporter /splunk_exporter

EXPOSE 9115

CMD [ "/splunk_exporter" ]