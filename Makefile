default: build

build:
	go build -o wnfs-sync ./cmd

install:
	go build -o wnfs-sync ./cmd
	cp wnfs-sync /usr/local/bin/wnfs-sync