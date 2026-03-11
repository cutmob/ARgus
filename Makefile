.PHONY: build run test clean deps frontend docker deploy deploy-cloudbuild logs

# Backend
build:
	go build -o argus-server ./cmd/server

run:
	go run ./cmd/server

deps:
	go mod tidy
	go mod download

test:
	go test ./...

# Frontend
frontend-install:
	cd frontend/web-client && npm install

frontend-dev:
	cd frontend/web-client && npm run dev

frontend-build:
	cd frontend/web-client && npm run build

# Landing Page
landing-install:
	cd landing && npm install

landing-dev:
	cd landing && npm run dev

landing-build:
	cd landing && npm run build

# Docker
docker-build:
	docker build -t argus-backend .

docker-run:
	docker run -p 8080:8080 --env-file .env argus-backend

# Development
dev: deps
	go run ./cmd/server

# Clean
clean:
	rm -f argus-server
	rm -rf reports/
	rm -rf frontend/web-client/.next
	rm -rf frontend/web-client/node_modules

# ── Cloud Run Deployment ──────────────────────────────────────────────────────
# Usage: make deploy
#        make deploy PROJECT=my-gcp-project-id
#        make deploy PROJECT=my-gcp-project-id REGION=europe-west1

PROJECT ?=
REGION  ?= us-central1

deploy:
	@bash scripts/deploy/deploy.sh \
		$(if $(PROJECT),--project $(PROJECT)) \
		$(if $(REGION),--region $(REGION))

# Submit a Cloud Build job manually (IaC path — mirrors CI/CD trigger)
deploy-cloudbuild:
	gcloud builds submit --config=cloudbuild.yaml \
		$(if $(PROJECT),--project=$(PROJECT)) \
		.

# Stream live logs from the deployed Cloud Run service
logs:
	gcloud run services logs tail argus-backend \
		--region=$(REGION) \
		$(if $(PROJECT),--project=$(PROJECT))

# ── Add new inspection module ─────────────────────────────────────────────────
# Usage: make new-module NAME=warehouse
new-module:
	@mkdir -p modules/$(NAME)
	@echo '[]' > modules/$(NAME)/rules.json
	@echo '{"author":"","version":"1","tags":[]}' > modules/$(NAME)/metadata.json
	@echo 'You are ARGUS, inspecting a $(NAME).' > modules/$(NAME)/prompt.txt
	@echo "Created module: modules/$(NAME)"
