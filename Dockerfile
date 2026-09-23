# Builds any SyMon component. Pick one with --target, for example:
#   docker build --target collector -t symon-collector .
# docker-compose.yml uses this to run the whole stack for development.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN for c in agent collector alertprocessor client; do \
        CGO_ENABLED=0 go build -o /out/$c ./$c || exit 1; \
    done

FROM alpine:3.22 AS assets
COPY client/fetch-assets.sh /fetch-assets.sh
RUN sh /fetch-assets.sh /frontend

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
COPY client/frontend ./frontend
COPY --from=assets /frontend ./frontend
USER symon
EXPOSE 8080
CMD ["./client"]

FROM alpine:3.22 AS agent
RUN adduser -D -H symon
WORKDIR /app
COPY --from=build /out/agent ./agent
USER symon
CMD ["./agent"]
