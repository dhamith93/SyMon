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

build-client:
	cd client && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o client_linux_x86_64

pack-all: pack-collector pack-agent pack-alertprocessor pack-client

pack-collector: build-collector
	mkdir -p release/collector_linux_x86_64
	cp collector/collector_linux_x86_64 release/collector_linux_x86_64
	cp collector/init.sql release/collector_linux_x86_64
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

pack-client: build-client
	mkdir -p release/client_linux_x86_64
	cp client/client_linux_x86_64 release/client_linux_x86_64
	cp client/.env-example release/client_linux_x86_64
	cp -r client/frontend release/client_linux_x86_64
	cp client/Dockerfile release/client_linux_x86_64
	cd release/client_linux_x86_64/frontend/css && wget "https://cdn.jsdelivr.net/npm/flatpickr@4.6.13/dist/flatpickr.min.css"
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdn.jsdelivr.net/npm/axios@1.2.2/dist/axios.min.js"
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdn.jsdelivr.net/npm/flatpickr@4.6.13/dist/flatpickr.min.js"
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdnjs.cloudflare.com/ajax/libs/Chart.js/3.5.1/chart.min.js"
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdnjs.cloudflare.com/ajax/libs/hammer.js/2.0.8/hammer.min.js"
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdnjs.cloudflare.com/ajax/libs/chartjs-plugin-zoom/1.1.1/chartjs-plugin-zoom.min.js"
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdn.jsdelivr.net/npm/moment@2.27.0" -O moment.min.js
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdn.jsdelivr.net/npm/moment-timezone@0.5.17/builds/moment-timezone-with-data.min.js"
	cd release/client_linux_x86_64/frontend/scripts && wget "https://cdn.jsdelivr.net/npm/chartjs-adapter-moment@0.1.1" -O chartjs-adapter-moment.min.js
	cd release/ && tar -cvf client_linux_x86_64.tar.gz client_linux_x86_64
	rm -rf release/client_linux_x86_64