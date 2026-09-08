APP_ENV ?= development
ENV_FILE ?= .env.$(APP_ENV)
LOCAL_ENV_FILE ?=$(ENV_FILE).local
RUNTIME_ENV_FILE ?=$(ENV_FILE)

ifeq ($(APP_ENV),development)
ifneq ($(wildcard $(LOCAL_ENV_FILE)),)
RUNTIME_ENV_FILE := $(LOCAL_ENV_FILE)
endif
endif

-include $(ENV_FILE)
-include $(LOCAL_ENV_FILE)
export

VERSION := $(strip $(shell cat VERSION))
LDFLAGS := -s -w -X github.com/endge-lab/service-mock-generator/internal/buildinfo.Version=$(VERSION)
MAIN := ./cmd/main.go
BIN := ./tmp/service-mock-generator

.PHONY: all
all: mod build test

.PHONY: mod
mod:
	go mod tidy

.PHONY: build
build:
	go build -ldflags="$(LDFLAGS)" -buildvcs=false -o $(BIN) $(MAIN)

.PHONY: run
run:
	APP_ENV=$(APP_ENV) go run -ldflags="$(LDFLAGS)" $(MAIN)

.PHONY: clean
clean:
	rm -rf tmp

.PHONY: test
test:
	go test -v ./...

.PHONY: up
up:
	APP_RUNTIME_ENV_FILE=$(RUNTIME_ENV_FILE) docker compose --env-file $(RUNTIME_ENV_FILE) up --build

.PHONY: down
down:
	APP_RUNTIME_ENV_FILE=$(RUNTIME_ENV_FILE) docker compose --env-file $(RUNTIME_ENV_FILE) down

.PHONY: proto contract-check race fuzz harness
proto:
	python3 scripts/proto.py --write
contract-check:
	python3 scripts/proto.py
race:
	go test -race ./...
fuzz:
	go test ./internal/usecase/generate -run '^$$' -fuzz '^FuzzSchema$$' -fuzztime=60s -parallel=2
	go test ./internal/usecase/generate -run '^$$' -fuzz '^FuzzPattern$$' -fuzztime=60s -parallel=2
	go test ./internal/usecase/generate -run '^$$' -fuzz '^FuzzRelations$$' -fuzztime=60s -parallel=2
	go test ./internal/usecase/stream -run '^$$' -fuzz '^FuzzCommands$$' -fuzztime=60s -parallel=2
harness:
	go build -o ./tmp/mock-harness ./test/harness
	cd ../egorkozelskij-endge-service-backend && go build -o ./tmp/mock-backend-harness ./test/mock-harness
	python3 test/harness/adversarial.py --mock-bin ./tmp/mock-harness --backend-bin ../egorkozelskij-endge-service-backend/tmp/mock-backend-harness --duration 900 --cycles 1000 --output ./tmp/adversarial-report.json
