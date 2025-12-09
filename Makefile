APP_CLI=georaw
APP_GUI=georaw-gui
BINDIR=bin

.PHONY: all cli-linux cli-windows gui-linux gui-windows clean

all: cli-linux

cli-linux:
	mkdir -p $(BINDIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(APP_CLI) ./cmd/georaw

cli-windows:
	mkdir -p $(BINDIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o $(BINDIR)/$(APP_CLI).exe ./cmd/georaw

gui-linux:
	mkdir -p $(BINDIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -tags dev -o $(BINDIR)/$(APP_GUI) ./cmd/georaw-gui

gui-windows:
	mkdir -p $(BINDIR)
	GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -tags dev -o $(BINDIR)/$(APP_GUI).exe ./cmd/georaw-gui
	@echo "Note: building GUI for Windows may require Mingw/CGO toolchain and WebView2 SDK."

clean:
	rm -rf $(BINDIR)
