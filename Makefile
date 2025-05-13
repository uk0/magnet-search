

build:
	@echo "Building.. search_linux"
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags '-extldflags "-static"' -o magnet-search-crawler cmd/crawler/crawler_main.go
	@echo "Building.."
	go build -o magnet-search-web cmd/server/web_main.go
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags '-extldflags "-static"' -o magnet-search-web-amd64  cmd/server/web_main.go
	@echo "Build done ...."