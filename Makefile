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

LDFLAGS := -s -w
MAIN := ./cmd/main.go
BIN := ./tmp/service-template

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
	APP_ENV=$(APP_ENV) go run $(MAIN)

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
