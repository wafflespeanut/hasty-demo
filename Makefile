IMAGE = hasty_service
PORT = 3000

all: build test

build:
	cd service && go build

image:
	cd service && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo
	-docker rmi -f $(IMAGE)
	docker build -t $(IMAGE) .

run: image
	-docker rm -f hasty
	docker run -it -p $(PORT):$(PORT) --name hasty -e PORT=$(PORT) $(IMAGE)

test:
	cd service && go test
