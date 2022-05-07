clean:
	echo "Removing binaries"
	rm -f agent/agent_linux_x86_64
	rm -f alertprocessor/alert_linux_x86_64
	rm -f client/client_linux_x86_64
	rm -f collector/collector_linux_x86_64

build:
	cd agent && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o agent_linux_x86_64
	cd alertprocessor && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o alert_linux_x86_64
	cd client && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o client_linux_x86_64
	cd collector && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o collector_linux_x86_64