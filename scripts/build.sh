docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t benz1/metadata2gd:latest \
  -t benz1/metadata2gd:v4.03 \
  --push .