.PHONY: build run clean

build:
	go build -o gocluster cmd/gocluster/main.go

run: build
	./gocluster

clean:
	rm -f gocluster

linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "-w" -o gocluster cmd/gocluster/main.go
