default: build

build:
	go build -v ./...

install:
	go install -v ./...

lint:
	golangci-lint config verify
	golangci-lint run

generate:
	go generate ./...

fmt:
	gofmt -s -w .

test:
	go test -v -count=1 -parallel=4 ./...

testacc:
	TF_ACC=1 go test -v -count=1 -parallel=4 -timeout 30m ./...

CLUSTER_NAME  := kargo-dev
KIND_CONFIG   := test/kind-config.yaml
KARGO_VALUES  := test/kargo-values.yaml
KARGO_CHART   := oci://ghcr.io/akuity/kargo-charts/kargo

devenv-up: cluster cert-manager argocd kargo devenv-status ## Full local env setup

cluster: ## Create Kind cluster
	@kind get clusters 2>/dev/null | grep -q '^$(CLUSTER_NAME)$$' \
		&& echo "Kind cluster '$(CLUSTER_NAME)' already exists" \
		|| kind create cluster --config $(KIND_CONFIG)
	@kubectl config use-context kind-$(CLUSTER_NAME)

cert-manager: ## Install cert-manager
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
	kubectl wait --for=condition=Available deployment/cert-manager -n cert-manager --timeout=120s
	kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
	kubectl wait --for=condition=Available deployment/cert-manager-cainjector -n cert-manager --timeout=120s

argocd: ## Install ArgoCD (--server-side required: applicationsets CRD exceeds client-side annotation limit)
	kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -n argocd --server-side --force-conflicts \
		-f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	kubectl wait --for=condition=Available deployment/argocd-server -n argocd --timeout=300s

kargo: ## Install Kargo via Helm (OCI registry)
	kubectl create namespace kargo --dry-run=client -o yaml | kubectl apply -f -
	helm upgrade --install kargo $(KARGO_CHART) \
		--namespace kargo \
		--values $(KARGO_VALUES) \
		--wait --timeout 300s

devenv-status: ## Show cluster status and env vars
	@echo ""
	@echo "Kargo UI:  https://localhost:31443  (admin / admin)"
	@echo "ArgoCD UI: kubectl port-forward -n argocd svc/argocd-server 8080:443"
	@echo "           https://localhost:8080  (admin / $$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d))"
	@echo ""
	@echo "export KARGO_API_URL=https://localhost:31443"
	@echo "export KARGO_ADMIN_PASSWORD=admin"
	@echo "export KARGO_INSECURE_SKIP_TLS_VERIFY=true"
	@echo "export TF_ACC=1"
	@echo ""
	@kubectl get pods -n kargo

ci: lint test ## Run what CI runs (lint + unit tests)
	@go mod tidy && git diff --exit-code go.mod go.sum
	@go vet ./...
	@test -z "$$(gofmt -l .)"

devenv-down: ## Tear down Kind cluster
	kind delete cluster --name $(CLUSTER_NAME)

.PHONY: build install lint generate fmt test testacc ci \
	devenv-up cluster cert-manager argocd kargo devenv-status devenv-down
