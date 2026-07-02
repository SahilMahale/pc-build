.PHONY: build run stop clean restart logs rebuild

IMAGE_NAME = pc-price-server
CONTAINER_NAME = pc-price-server
PORT = 8090

build:
	podman build -t $(IMAGE_NAME) .

run:
	mkdir -p data
	podman run -d \
		-p $(PORT):$(PORT) \
		-v ./config.yaml:/app/config.yaml:ro \
		-v ./data:/data \
		--name $(CONTAINER_NAME) \
		$(IMAGE_NAME)

stop:
	podman stop $(CONTAINER_NAME) || true
	podman rm $(CONTAINER_NAME) || true

clean: stop
	podman rmi $(IMAGE_NAME) || true

restart: stop run

logs:
	podman logs -f $(CONTAINER_NAME)

stats:
	podman stats $(CONTAINER_NAME)

inspect:
	podman inspect $(CONTAINER_NAME)

rebuild: stop build run

dev:
	go run .

test:
	go test ./...
