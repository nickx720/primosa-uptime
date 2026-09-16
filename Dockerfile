# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -o /primosa-uptime .

FROM gcr.io/distroless/static
COPY --from=build /primosa-uptime /primosa-uptime
COPY targets.json /app/targets.json
WORKDIR /app
EXPOSE 8080
ENTRYPOINT ["/primosa-uptime"]
