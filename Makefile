# ─────────────────────────────────────────────────────────────────────────────
# eLearning Platform — KinD Dev Environment
# ─────────────────────────────────────────────────────────────────────────────

KIND_CLUSTER   := elearning
HELM_DIR       := helm
HELM_RELEASE   := elearning
HELM_VALUES    := infra/kind/kind-values.yaml
PORT_FWDS_LOG  := /tmp/elearning-port-forwards.log
COURSE_DIR     := examples/courses

REGISTRY       := ghcr.io/towarmat83
IMAGE_COURSE   := $(REGISTRY)/elearning-course-service:latest
IMAGE_USER     := $(REGISTRY)/elearning-user-service:latest
IMAGE_FRONTEND := $(REGISTRY)/elearning-frontend:latest

# ── KinD ────────────────────────────────────────────────────────────────────

.PHONY: kind-create
kind-create:
	kind create cluster --name $(KIND_CLUSTER) --config infra/kind/kind-config.yaml

.PHONY: kind-delete
kind-delete:
	-kind delete cluster --name $(KIND_CLUSTER)

# ── Docker ───────────────────────────────────────────────────────────────────

.PHONY: docker-build docker-build-course docker-build-user docker-build-frontend

docker-build: docker-build-course docker-build-user docker-build-frontend

docker-build-course:
	docker build -t $(IMAGE_COURSE) course-service

docker-build-user:
	docker build -t $(IMAGE_USER) user-service

docker-build-frontend:
	docker build -t $(IMAGE_FRONTEND) frontend

# ── Kind load ───────────────────────────────────────────────────────────────

.PHONY: kind-load kind-load-course kind-load-user kind-load-frontend

kind-load: kind-load-course kind-load-user kind-load-frontend

kind-load-course:
	kind load docker-image $(IMAGE_COURSE) --name $(KIND_CLUSTER)

kind-load-user:
	kind load docker-image $(IMAGE_USER) --name $(KIND_CLUSTER)

kind-load-frontend:
	kind load docker-image $(IMAGE_FRONTEND) --name $(KIND_CLUSTER)

# ── Quick rebuild + reload (single service) ────────────────────────────────

.PHONY: rebuild rebuild-course rebuild-user rebuild-frontend

rebuild: rebuild-course rebuild-user rebuild-frontend

rebuild-course: docker-build-course kind-load-course
	kubectl rollout restart deployment/$(HELM_RELEASE)-course-service
	kubectl rollout status deployment/$(HELM_RELEASE)-course-service --timeout=120s

rebuild-user: docker-build-user kind-load-user
	kubectl rollout restart deployment/$(HELM_RELEASE)-user-service
	kubectl rollout status deployment/$(HELM_RELEASE)-user-service --timeout=120s

rebuild-frontend: docker-build-frontend kind-load-frontend
	kubectl rollout restart deployment/$(HELM_RELEASE)-frontend
	kubectl rollout status deployment/$(HELM_RELEASE)-frontend --timeout=120s

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

# ── Courses ──────────────────────────────────────────────────────────────────

.PHONY: apply-courses
apply-courses:
	@for f in $(COURSE_DIR)/*/course.yaml; do \
		echo "Applying $$f..."; \
		kubectl apply -f "$$f"; \
	done

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
	kubectl port-forward svc/$(HELM_RELEASE)-course-service 18082:8082 &
	kubectl port-forward svc/$(HELM_RELEASE)-user-service  18081:8081 &
	kubectl port-forward svc/$(HELM_RELEASE)-frontend      3000:3000 &
	sleep 2
	@echo "Port-forwards started (course:18082, user:18081, frontend:3000)"

.PHONY: port-forward-stop
port-forward-stop:
	-pkill -f "kubectl port-forward" 2>/dev/null
	@echo "Port-forwards stopped"

# ── Full lifecycle ────────────────────────────────────────────────────────────

.PHONY: dev
dev: kind-delete kind-create docker-build kind-load helm-install apply-courses
	@echo ""
	@echo "Waiting for deployments to be ready..."
	@kubectl rollout status deploy/$(HELM_RELEASE)-course-service --timeout=120s
	@kubectl rollout status deploy/$(HELM_RELEASE)-user-service  --timeout=120s
	@kubectl rollout status deploy/$(HELM_RELEASE)-frontend      --timeout=120s
	@kubectl rollout status deploy/elearning-postgresql          --timeout=60s
	@echo ""
	@echo "=== Deployment ready ==="
	@echo "Run 'make port-forward' to expose services locally."
	@echo ""
	@echo "  Admin login: admin@elearning.local / Admin@1234"
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
	@echo "=== frontend ==="
	@kubectl logs deploy/$(HELM_RELEASE)-frontend --tail=10 2>&1
	@echo ""
	@echo "=== postgresql ==="
	@kubectl logs deploy/elearning-postgresql --tail=5 2>&1 || true

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

# ── Status ──────────────────────────────────────────────────────────────────

.PHONY: status
status:
	@kubectl get pods
	@echo ""
	@kubectl get svc
