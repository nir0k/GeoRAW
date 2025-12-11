APP_CLI=georaw-cli
APP_GUI=georaw-gui
APP_SERIES=georaw-series-cli
BINDIR=bin

VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS=-X github.com/nir0k/GeoRAW/internal/version.Version=$(VERSION)

.PHONY: all cli-linux cli-windows gui-linux gui-windows series-linux series-windows clean

all: cli-linux series-linux gui-linux cli-windows series-windows

cli-linux:
	mkdir -p $(BINDIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(APP_CLI).linux-amd64 ./cmd/georaw

cli-windows:
	mkdir -p $(BINDIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(APP_CLI).exe ./cmd/georaw

gui-linux:
	mkdir -p $(BINDIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags production -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(APP_GUI).linux-amd64 ./cmd/georaw-gui

gui-windows:
	mkdir -p $(BINDIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -tags production -ldflags="-H windowsgui $(LDFLAGS)" -o $(BINDIR)/$(APP_GUI).exe ./cmd/georaw-gui
	@echo "Note: building GUI for Windows may require Mingw/CGO toolchain and WebView2 SDK."

series-linux:
	mkdir -p $(BINDIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(APP_SERIES).linux-amd64 ./cmd/georaw-series

series-windows:
	mkdir -p $(BINDIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/$(APP_SERIES).exe ./cmd/georaw-series

clean:
	rm -rf $(BINDIR)
