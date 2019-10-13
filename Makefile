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
	-docker network create roachnet
	-docker rm -f roach1
	docker run -d --name roach1 --hostname=roach1 --network roachnet \
		-p 26257:26257 cockroachdb/cockroach start --insecure
	-docker rm -f hasty
	docker run -it --network roachnet -e ACCESS_TOKEN=$(ACCESS_TOKEN) \
		-e POSTGRES_URL=postgresql://root@roach1:26257?sslmode=disable \
		-p $(PORT):$(PORT) --name hasty $(IMAGE) -p $(PORT)

test:
	cd service && go test
