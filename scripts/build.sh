docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t benz1/meta2cloud:latest \
  -t benz1/meta2cloud:v4.03 \
  --push .
