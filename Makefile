.PHONY: build run stop clean restart logs rebuild deploy-ibm

IMAGE_NAME = pc-price-server
CONTAINER_NAME = pc-price-server
PORT = 8090
IBM_PROJECT = pc-price-tracker
IBM_APP = pc-price-server

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

# IBM Cloud Code Engine deployment
deploy-ibm:
	@echo "Deploying to IBM Cloud Code Engine..."
	ibmcloud ce project select --name $(IBM_PROJECT) || ibmcloud ce project create --name $(IBM_PROJECT)
	ibmcloud ce configmap create --name pc-config --from-file config.yaml --force || \
		ibmcloud ce configmap update --name pc-config --from-file config.yaml
	ibmcloud ce app create --name $(IBM_APP) \
		--image docker.io/sahilmahale/$(IMAGE_NAME):latest \
		--port $(PORT) \
		--min-scale 1 --max-scale 1 \
		--cpu 0.25 --memory 0.5G \
		--mount-configmap /app/config.yaml=pc-config \
		--env-from-configmap pc-config || \
	ibmcloud ce app update --name $(IBM_APP) \
		--image docker.io/sahilmahale/$(IMAGE_NAME):latest
	@echo "Deployment complete. Run 'make ibm-url' to get the public URL."

ibm-url:
	@ibmcloud ce app get --name $(IBM_APP) --output url

ibm-logs:
	ibmcloud ce app logs --name $(IBM_APP) --follow

ibm-delete:
	ibmcloud ce app delete --name $(IBM_APP) --force
	ibmcloud ce configmap delete --name pc-config --force

push-dockerhub:
	docker tag $(IMAGE_NAME) docker.io/sahilmahale/$(IMAGE_NAME):latest
	docker push docker.io/sahilmahale/$(IMAGE_NAME):latest
