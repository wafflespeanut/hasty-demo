IMAGE = hasty_service
PORT = 3000
ACCESS_TOKEN = foobar

all: build test

build:
	cd service && go build

image:
	cd service && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo
	-docker rmi -f $(IMAGE)
	docker build -t $(IMAGE) .

run: image
	-docker rm -f hasty
	docker run -it -e ACCESS_TOKEN=$(ACCESS_TOKEN) -p $(PORT):$(PORT) --name hasty $(IMAGE) -p $(PORT)

test:
	cd service && go test
