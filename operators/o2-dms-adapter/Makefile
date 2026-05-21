# O2 DMS Adapter Makefile (P9-T-008 scaffold).

.PHONY: build test vet lint image helm-lint helm-template clean

build:
	go build ./...

test:
	go test ./... -count=1 -timeout 120s

vet:
	go vet ./...

lint: vet

image:
	docker build -t o2-dms-adapter:dev .

helm-lint:
	helm lint --strict ../../deploy/helm-charts/o2-dms-adapter

helm-template:
	helm template test ../../deploy/helm-charts/o2-dms-adapter

clean:
	go clean -cache
	rm -rf bin
