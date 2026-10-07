.PHONY: init deps dev backend web check integration browser recovery image up backup
init:
	python3 scripts/init-env.py
	docker compose up -d --wait postgres mailpit
deps:
	go mod download
	cd web && npm ci
backend:
	python3 scripts/run-env.py --owner go run ./cmd/tenantforge migrate
	python3 scripts/run-env.py go run ./cmd/tenantforge serve
web:
	cd web && npm run dev
dev:
	python3 scripts/dev.py
check:
	test -z "$$(gofmt -l cmd internal migrations extension examples)"
	go test -race -timeout 120s ./...
	go vet ./...
	cd web && npm run format:check && npm test && npm run build
integration:
	python3 scripts/run-env.py --owner go test -race -count=1 -timeout 180s ./...
browser:
	python3 scripts/browser-check.py
recovery:
	python3 scripts/recovery-check.py
image:
	docker compose --profile app build
up:
	python3 scripts/init-env.py
	docker compose --profile app up -d --build
backup:
	python3 scripts/backup.py
