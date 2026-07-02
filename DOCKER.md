# pc-price-server

Build and run:
```bash
podman build -t pc-price-server .
podman run -d \
  -p 8090:8090 \
  -v ./config.yaml:/app/config.yaml:ro \
  -v ./data:/data \
  --name pc-price-server \
  pc-price-server
```

Or with Docker:
```bash
docker build -t pc-price-server .
docker run -d \
  -p 8090:8090 \
  -v ./config.yaml:/app/config.yaml:ro \
  -v ./data:/data \
  --name pc-price-server \
  pc-price-server
```

The database (`prices.db`) will be stored in the `./data` volume.

Environment variables:
- `HOST` (default: `0.0.0.0`)
- `PORT` (default: `8090`)
