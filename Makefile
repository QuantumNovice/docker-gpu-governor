IMAGE ?= quantumnovice/gpu-governor
TAG ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo 0.0.0)
PLATFORMS ?= linux/amd64,linux/arm64
BUILDER ?= gpu-governor-builder

.PHONY: build install update uninstall validate test ui dev-ui dev-reset push builder

build: ## Build the extension image for this machine
	docker build --build-arg VERSION=$(TAG) -t $(IMAGE):$(TAG) .

install: build ## Build and install the extension into Docker Desktop
	docker extension install $(IMAGE):$(TAG) -f

update: build ## Rebuild and update an installed extension
	docker extension update $(IMAGE):$(TAG) -f

uninstall:
	docker extension rm $(IMAGE)

validate: ## Run the marketplace checks against the pushed image
	docker extension validate $(IMAGE):$(TAG)

test: ## Run Go and UI checks
	sh scripts/gotest.sh
	cd ui && npm ci && npm run build

dev-ui: ## Serve the UI with hot reload inside Docker Desktop
	docker extension dev ui-source $(IMAGE) http://localhost:3000
	cd ui && npm run dev

dev-reset:
	docker extension dev reset $(IMAGE)

builder:
	docker buildx inspect $(BUILDER) >/dev/null 2>&1 || docker buildx create --name $(BUILDER) --driver docker-container

push: builder ## Build for amd64 and arm64 and push to Docker Hub
	docker buildx build --builder $(BUILDER) --push --platform $(PLATFORMS) \
		--build-arg VERSION=$(TAG) -t $(IMAGE):$(TAG) -t $(IMAGE):latest .
