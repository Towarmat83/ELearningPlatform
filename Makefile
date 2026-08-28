# ─────────────────────────────────────────────────────────────────────────────
# Pupitre Platform — KinD Dev Environment
# ─────────────────────────────────────────────────────────────────────────────

KIND_CLUSTER   ?= pupitre
HELM_DIR       := helm
HELM_RELEASE   ?= pupitre

# Local overrides (git-ignored) — create local.mk to set KIND_CLUSTER, HELM_RELEASE, etc.
-include local.mk
HELM_VALUES    := infra/kind/kind-values.yaml
PORT_FWDS_LOG  := /tmp/pupitre-port-forwards.log
DOCKER         ?= podman

REGISTRY        := ghcr.io/genesary
IMAGE_COURSE    := localhost/pupitre-course-service:local
IMAGE_USER      := localhost/pupitre-user-service:local
IMAGE_FRONTEND  := localhost/pupitre-frontend:local
IMAGE_CHECKER   := localhost/pupitre-checker-service:local

# ── KinD ────────────────────────────────────────────────────────────────────

.PHONY: kind-create
kind-create:
	kind create cluster --name $(KIND_CLUSTER) --config infra/kind/kind-config.yaml

.PHONY: kind-delete
kind-delete:
	-kind delete cluster --name $(KIND_CLUSTER)

# ── Docker ───────────────────────────────────────────────────────────────────

.PHONY: docker-build docker-build-course docker-build-user docker-build-frontend docker-build-checker

docker-build: docker-build-course docker-build-user docker-build-frontend docker-build-checker

docker-build-course:
	$(DOCKER) build -t $(IMAGE_COURSE) course-service

docker-build-user:
	$(DOCKER) build -t $(IMAGE_USER) user-service

docker-build-frontend:
	$(DOCKER) build -t $(IMAGE_FRONTEND) frontend

docker-build-checker:
	$(DOCKER) build -t $(IMAGE_CHECKER) checker-service

# ── Kind load ───────────────────────────────────────────────────────────────

.PHONY: kind-load kind-load-course kind-load-user kind-load-frontend kind-load-checker

kind-load: kind-load-course kind-load-user kind-load-frontend kind-load-checker

kind-load-course:
	$(DOCKER) save $(IMAGE_COURSE) | kind load image-archive /dev/stdin --name $(KIND_CLUSTER)

kind-load-user:
	$(DOCKER) save $(IMAGE_USER) | kind load image-archive /dev/stdin --name $(KIND_CLUSTER)

kind-load-frontend:
	$(DOCKER) save $(IMAGE_FRONTEND) | kind load image-archive /dev/stdin --name $(KIND_CLUSTER)

kind-load-checker:
	$(DOCKER) save $(IMAGE_CHECKER) | kind load image-archive /dev/stdin --name $(KIND_CLUSTER)

# ── Quick rebuild + reload (single service) ────────────────────────────────

.PHONY: rebuild rebuild-course rebuild-user rebuild-frontend rebuild-checker

rebuild: rebuild-course rebuild-user rebuild-frontend rebuild-checker

rebuild-course: docker-build-course kind-load-course
	kubectl rollout restart deployment/$(HELM_RELEASE)-course-service
	kubectl rollout status deployment/$(HELM_RELEASE)-course-service --timeout=120s

rebuild-user: docker-build-user kind-load-user
	kubectl rollout restart deployment/$(HELM_RELEASE)-user-service
	kubectl rollout status deployment/$(HELM_RELEASE)-user-service --timeout=120s

rebuild-frontend: docker-build-frontend kind-load-frontend
	kubectl rollout restart deployment/$(HELM_RELEASE)-frontend
	kubectl rollout status deployment/$(HELM_RELEASE)-frontend --timeout=120s

rebuild-checker: docker-build-checker kind-load-checker
	kubectl rollout restart deployment/$(HELM_RELEASE)-checker-service
	kubectl rollout status deployment/$(HELM_RELEASE)-checker-service --timeout=120s

# ── Helm ─────────────────────────────────────────────────────────────────────

.PHONY: helm-deps
helm-deps:
	helm dependency update $(HELM_DIR)

.PHONY: helm-install
helm-install:
	helm upgrade --install $(HELM_RELEASE) $(HELM_DIR) --values $(HELM_VALUES)

.PHONY: helm-delete
helm-delete:
	-helm uninstall $(HELM_RELEASE)

# ── Dev courses ──────────────────────────────────────────────────────────────
#
# The demo catalogue lives in course-service/internal/db/seed/courses/ and is
# embedded in the binary. course-service seeds it on startup when
# SEED_DEV_COURSES is set, which the KinD values files already do.

.PHONY: seed-courses
seed-courses:
	@echo "Re-seeding dev courses (overwrite mode)..."
	kubectl set env deploy/$(HELM_RELEASE)-course-service SEED_DEV_COURSES=overwrite
	kubectl rollout status deploy/$(HELM_RELEASE)-course-service --timeout=120s
	kubectl set env deploy/$(HELM_RELEASE)-course-service SEED_DEV_COURSES=true
	@echo "Dev courses re-seeded from the embedded definitions."

# ── Git secret ───────────────────────────────────────────────────────────────

.PHONY: create-git-secret
create-git-secret:
	@echo "Create a git-credentials.yaml file first, then run:"
	@echo "  kubectl create secret generic course-repo-secret \\"
	@echo "    --from-file=git-credentials.yaml=./git-credentials.yaml"
	@echo ""
	@echo "See infra/examples/course-service/course-secret.yaml for the format."

# ── Port-forwards ───────────────────────────────────────────────────────────

.PHONY: port-forward
port-forward:
	kubectl port-forward svc/$(HELM_RELEASE)-course-service  18082:8082 &
	kubectl port-forward svc/$(HELM_RELEASE)-user-service   18081:8081 &
	kubectl port-forward svc/$(HELM_RELEASE)-frontend       3000:3000  &
	kubectl port-forward svc/$(HELM_RELEASE)-checker-service 18083:8083 &
	sleep 2
	@echo "Port-forwards started (course:18082, user:18081, frontend:3000, checker:18083)"

.PHONY: port-forward-stop
port-forward-stop:
	-pkill -f "kubectl port-forward" 2>/dev/null
	@echo "Port-forwards stopped"

# ── Full lifecycle ────────────────────────────────────────────────────────────

.PHONY: dev
dev: kind-delete kind-create docker-build kind-load helm-install
	@echo ""
	@echo "Waiting for deployments to be ready..."
	@kubectl rollout status deploy/$(HELM_RELEASE)-course-service  --timeout=120s
	@kubectl rollout status deploy/$(HELM_RELEASE)-user-service   --timeout=120s
	@kubectl rollout status deploy/$(HELM_RELEASE)-frontend       --timeout=120s
	@kubectl rollout status deploy/$(HELM_RELEASE)-checker-service --timeout=120s
	@kubectl rollout status statefulset/$(HELM_RELEASE)-postgresql --timeout=60s
	@echo ""
	@echo "=== Deployment ready ==="
	@echo "Run 'make port-forward' to expose services locally."
	@echo ""
	@echo "  Admin login: admin@pupitre.local"
	@echo "  Password:    kubectl get secret $(HELM_RELEASE)-secrets \\"
	@echo "                 -o jsonpath='{.data.ADMIN_PASSWORD}' | base64 -d"
	@echo "  Courses:     17 demo courses seeded (SEED_DEV_COURSES=true)"
	@echo "  Frontend:    http://localhost:3000"
	@echo "  Course API:  http://localhost:18082"
	@echo "  User API:    http://localhost:18081"
	@echo ""
	@echo "Next: create a git secret if you use private repos:"
	@echo "  make create-git-secret"

.PHONY: clean
clean: helm-delete kind-delete
	@echo "Cleaned up"

# ── Logs ────────────────────────────────────────────────────────────────────

.PHONY: logs
logs:
	@echo "=== course-service ==="
	@kubectl logs deploy/$(HELM_RELEASE)-course-service --tail=20 2>&1 | grep -v health
	@echo ""
	@echo "=== user-service ==="
	@kubectl logs deploy/$(HELM_RELEASE)-user-service --tail=20 2>&1 | grep -v health
	@echo ""
	@echo "=== checker-service ==="
	@kubectl logs deploy/$(HELM_RELEASE)-checker-service --tail=20 2>&1 | grep -v health
	@echo ""
	@echo "=== frontend ==="
	@kubectl logs deploy/$(HELM_RELEASE)-frontend --tail=10 2>&1
	@echo ""
	@echo "=== postgresql ==="
	@kubectl logs statefulset/$(HELM_RELEASE)-postgresql --tail=5 2>&1 || true

# ── OpenAPI ──────────────────────────────────────────────────────────────────
#
# Prerequisites:
#   go install github.com/swaggo/swag/cmd/swag@latest

.PHONY: openapi-gen openapi-gen-course-service openapi-gen-user-service

openapi-gen: openapi-gen-course-service openapi-gen-user-service
	@echo "OpenAPI JSON files regenerated from code."

openapi-gen-course-service:
	@which swag > /dev/null 2>&1 || (echo "swag not found — run: go install github.com/swaggo/swag/cmd/swag@latest" && exit 1)
	@echo "Generating course-service/openapi.json from code..."
	@cd course-service && swag init -g main.go --output . --outputTypes json --parseInternal --quiet && mv swagger.json openapi.json

openapi-gen-user-service:
	@which swag > /dev/null 2>&1 || (echo "swag not found — run: go install github.com/swaggo/swag/cmd/swag@latest" && exit 1)
	@echo "Generating user-service/openapi.json from code..."
	@cd user-service && swag init -g main.go --output . --outputTypes json --parseInternal --quiet && mv swagger.json openapi.json

# ── Go Tests ────────────────────────────────────────────────────────────────

.PHONY: go/test go/test-course go/test-user

go/test: go/test-course go/test-user go/test-checker

go/test-course:
	@echo "=== course-service tests ==="
	@cd course-service && CGO_ENABLED=1 go test ./... -race -coverprofile=coverage.out -count=1 && go tool cover -func=coverage.out | tail -1

go/test-user:
	@echo "=== user-service tests ==="
	@cd user-service && CGO_ENABLED=1 go test ./... -race -coverprofile=coverage.out -count=1 && go tool cover -func=coverage.out | tail -1

go/test-checker:
	@echo "=== checker-service tests ==="
	@cd checker-service && CGO_ENABLED=1 go test ./... -race -coverprofile=coverage.out -count=1 && go tool cover -func=coverage.out | tail -1

# ── Go Lint ─────────────────────────────────────────────────────────────────

.PHONY: go/lint go/lint-course go/lint-user go/lint-checker

go/lint: go/lint-course go/lint-user go/lint-checker

go/lint-course:
	@echo "=== Linting course-service ==="
	@cd course-service && golangci-lint run ./...

go/lint-user:
	@echo "=== Linting user-service ==="
	@cd user-service && golangci-lint run ./...

go/lint-checker:
	@echo "=== Linting checker-service ==="
	@cd checker-service && golangci-lint run ./...

# ── E2E Tests ───────────────────────────────────────────────────────────────
#
# Runs e2e tests against services already port-forwarded to localhost.
# Requires: USER_SERVICE_URL, COURSE_SERVICE_URL (optional), CHECKER_SERVICE_URL,
#           ADMIN_EMAIL, ADMIN_PASSWORD.
# Example:
#   USER_SERVICE_URL=http://localhost:8081 \
#   COURSE_SERVICE_URL=http://localhost:8082 \
#   CHECKER_SERVICE_URL=http://localhost:8083 \
#   ADMIN_PASSWORD=<password> \
#   make e2e/test

.PHONY: e2e/test
e2e/test:
	@echo "=== E2E tests ==="
	@cd e2e && go test -tags e2e -v -timeout 120s ./...

# ── Status ──────────────────────────────────────────────────────────────────

.PHONY: status
status:
	@kubectl get pods
	@echo ""
	@kubectl get svc
