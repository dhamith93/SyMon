.PHONY: proto web clean build-all build-collector build-agent build-alertprocessor build-client pack-all pack-collector pack-agent pack-alertprocessor pack-client

proto:
	cd internal && protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		api/api.proto alertapi/alertapi.proto

web:
	cd client/web && npm ci && npm run build

clean:	
	rm -f agent/agent_linux_x86_64
	rm -f alertprocessor/alertprocessor_linux_x86_64
	rm -f client/client_linux_x86_64
	rm -f collector/collector_linux_x86_64
	rm -f agent/agent
	rm -f alertprocessor/alerts
	rm -f client/client
	rm -f collector/collector
	rm -rf release
	rm -rf collector/release/
	rm -rf agent/release/
	rm -rf alertprocessor/release/
	rm -rf client/release/

build-all: build-collector build-agent build-alertprocessor build-client

build-collector:
	cd collector && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o collector_linux_x86_64

build-agent:
	cd agent && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o agent_linux_x86_64

build-alertprocessor:
	cd alertprocessor && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o alertprocessor_linux_x86_64

build-client: web
	cd client && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o client_linux_x86_64

pack-all: pack-collector pack-agent pack-alertprocessor pack-client

pack-collector: build-collector
	mkdir -p release/collector_linux_x86_64
	cp collector/collector_linux_x86_64 release/collector_linux_x86_64
	cp collector/.env-example release/collector_linux_x86_64
	cp collector/alerts.json release/collector_linux_x86_64
	cd release/ && tar -cvf collector_linux_x86_64.tar.gz collector_linux_x86_64
	rm -rf release/collector_linux_x86_64

pack-agent: build-agent
	mkdir -p release/agent_linux_x86_64
	cp agent/agent_linux_x86_64 release/agent_linux_x86_64
	cp agent/.env-example release/agent_linux_x86_64
	cd release/ && tar -cvf agent_linux_x86_64.tar.gz agent_linux_x86_64
	rm -rf release/agent_linux_x86_64

pack-alertprocessor: build-alertprocessor
	mkdir -p release/alertprocessor_linux_x86_64
	cp alertprocessor/alertprocessor_linux_x86_64 release/alertprocessor_linux_x86_64
	cp alertprocessor/.env-example release/alertprocessor_linux_x86_64
	cd release/ && tar -cvf alertprocessor_linux_x86_64.tar.gz alertprocessor_linux_x86_64
	rm -rf release/alertprocessor_linux_x86_64

# the agent builds are what new hosts download from the dashboard.
# GOARM=6 also runs on ARMv7, so one arm build covers every Raspberry Pi.
pack-client: build-client
	mkdir -p release/client_linux_x86_64/downloads
	cp client/client_linux_x86_64 release/client_linux_x86_64
	cd agent && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ../release/client_linux_x86_64/downloads/agent-linux-amd64
	cd agent && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o ../release/client_linux_x86_64/downloads/agent-linux-arm64
	cd agent && GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 go build -o ../release/client_linux_x86_64/downloads/agent-linux-arm
	cp client/.env-example release/client_linux_x86_64
	cp client/Dockerfile release/client_linux_x86_64
	cd release/ && tar -cvf client_linux_x86_64.tar.gz client_linux_x86_64
	rm -rf release/client_linux_x86_64
