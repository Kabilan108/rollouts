build/rollout: $(shell find . -name '*.go')
	go build -ldflags="-s -w" -o build/rollout .

build/rollout-linux-amd64: $(shell find . -name '*.go')
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o build/rollout-linux-amd64 .

build: build/rollout

install:
	go install

deps:
	go mod tidy

clean:
	rm -f build/rollout
	rm -rf rollout-linux-amd64
	rm -f rollout-linux-amd64.tar.gz

run: build
	./build/rollout

release: build/rollout-linux-amd64
	cp build/rollout-linux-amd64 rollout
	tar czf rollout-linux-amd64.tar.gz -C build rollout
	rm -rf rollout
