.PHONY: clean build-all build-collector build-agent build-alertprocessor build-client pack-all pack-collector pack-agent pack-alertprocessor pack-client

clean:	
	rm -f agent/agent_linux_x86_64
	rm -f alertprocessor/alertprocessor_linux_x86_64
	rm -f client/client_linux_x86_64
	rm -f collector/collector_linux_x86_64
	rm -f agent/agent
	rm -f alertprocessor/alerts
	rm -f client/client
	rm -f collector/collector
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

build-client:
	cd client && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o client_linux_x86_64

pack-all: pack-collector pack-agent pack-alertprocessor pack-client

pack-collector: build-collector
	mkdir -p collector/release/collector_linux_x86_64
	cp collector/collector_linux_x86_64 collector/release/collector_linux_x86_64
	cp collector/init.sql collector/release/collector_linux_x86_64
	cp collector/config_example.json collector/release/collector_linux_x86_64
	cp collector/.env-example collector/release/collector_linux_x86_64
	cp collector/alerts.json collector/release/collector_linux_x86_64
	cd collector/release/ && tar -cvf collector_linux_x86_64.tar.gz collector_linux_x86_64
	rm -rf collector/release/collector_linux_x86_64

pack-agent: build-agent
	mkdir -p agent/release/agent_linux_x86_64
	cp agent/agent_linux_x86_64 agent/release/agent_linux_x86_64
	cp agent/config-example.json agent/release/agent_linux_x86_64
	cd agent/release/ && tar -cvf agent_linux_x86_64.tar.gz agent_linux_x86_64
	rm -rf agent/release/agent_linux_x86_64

pack-alertprocessor: build-alertprocessor
	mkdir -p alertprocessor/release/alertprocessor_linux_x86_64
	cp alertprocessor/alertprocessor_linux_x86_64 alertprocessor/release/alertprocessor_linux_x86_64
	cp alertprocessor/config.json alertprocessor/release/alertprocessor_linux_x86_64
	cp alertprocessor/.env-example alertprocessor/release/alertprocessor_linux_x86_64
	cd alertprocessor/release/ && tar -cvf alertprocessor_linux_x86_64.tar.gz alertprocessor_linux_x86_64
	rm -rf alertprocessor/release/alertprocessor_linux_x86_64

pack-client: build-client
	mkdir -p client/release/client_linux_x86_64
	cp client/client_linux_x86_64 client/release/client_linux_x86_64
	cp client/config-example.json client/release/client_linux_x86_64
	cp -r client/frontend client/release/client_linux_x86_64
	cd client/release/ && tar -cvf client_linux_x86_64.tar.gz client_linux_x86_64
	rm -rf client/release/client_linux_x86_64