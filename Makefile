VERSION ?= 0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
PLATFORMS := darwin/arm64 darwin/amd64 linux/amd64

.PHONY: build run clean release deploy

build:
	go build $(LDFLAGS) -o forge-host ./cmd/forge-host
	go build -o forge-host-tray ./cmd/forge-host-tray

run: build
	./forge-host

clean:
	rm -f forge-host forge-host-* forge-host-tray forge-host-tray-*

deploy: build
	sudo cp forge-host /usr/local/bin/forge-host
	sudo launchctl kickstart -k system/dev.forge
	@echo "Deployed and restarted."

release:
	@for platform in $(PLATFORMS); do \
		os=$${platform%%/*}; \
		arch=$${platform##*/}; \
		echo "Building forge-host-$${os}-$${arch}..."; \
		GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o forge-host-$${os}-$${arch} ./cmd/forge-host; \
		echo "Building forge-host-tray-$${os}-$${arch}..."; \
		GOOS=$$os GOARCH=$$arch go build -o forge-host-tray-$${os}-$${arch} ./cmd/forge-host-tray; \
	done
	@echo "Release binaries built."
