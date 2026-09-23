# Builds any SyMon component. Pick one with --target, for example:
#   docker build --target collector -t symon-collector .
# docker-compose.yml uses this to run the whole stack for development.

FROM node:22-alpine AS web
WORKDIR /web
COPY client/web/package.json client/web/package-lock.json ./
RUN npm ci
COPY client/web ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./client/web/dist
RUN for c in agent collector alertprocessor client; do \
        CGO_ENABLED=0 go build -o /out/$c ./$c || exit 1; \
    done
# agent builds that new hosts download from the dashboard
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/downloads/agent-linux-amd64 ./agent \
    && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/downloads/agent-linux-arm64 ./agent \
    && CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6 go build -o /out/downloads/agent-linux-arm ./agent

FROM alpine:3.22 AS collector
RUN adduser -D -H symon
WORKDIR /app
COPY --from=build /out/collector ./collector
COPY collector/alerts.json ./
USER symon
EXPOSE 9000
CMD ["./collector"]

FROM alpine:3.22 AS alertprocessor
RUN adduser -D -H symon
WORKDIR /app
COPY --from=build /out/alertprocessor ./alertprocessor
USER symon
EXPOSE 9001
CMD ["./alertprocessor"]

FROM alpine:3.22 AS client
RUN adduser -D -H symon
WORKDIR /app
COPY --from=build /out/client ./client
COPY --from=build /out/downloads ./downloads
USER symon
EXPOSE 8080
CMD ["./client"]

FROM alpine:3.22 AS agent
RUN adduser -D -H symon
WORKDIR /app
COPY --from=build /out/agent ./agent
USER symon
CMD ["./agent"]
