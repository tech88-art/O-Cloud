# Makefile for configs/mock-data/generator.
#
# Targets:
#   gen-small    Generate set-a-small/
#   gen-multi    Generate set-b-multi-site/ (P1-T-307, returns error today)
#   gen-stress   Generate set-c-stress/    (P1-T-307, returns error today)
#   validate     Run ajv against all set-*/ JSON files
#   all          gen-small + validate
#
# Paths are relative to this directory (configs/mock-data/generator/).

SHELL := /bin/bash

# Project root (two parents up from this Makefile).
ROOT := $(realpath ../../..)
SCHEMA := $(ROOT)/configs/mock-data/schema.json

OUT_SMALL  := ../set-a-small
OUT_MULTI  := ../set-b-multi-site
OUT_STRESS := ../set-c-stress

.PHONY: gen-small gen-multi gen-stress validate all build vet test help

help:
	@echo "Targets: gen-small | gen-multi | gen-stress | validate | all | build | vet"

gen-small:
	go run . --preset small --output $(OUT_SMALL)/

gen-multi:
	go run . --preset multi --output $(OUT_MULTI)/

gen-stress:
	go run . --preset stress --output $(OUT_STRESS)/

build:
	go build ./...

vet:
	go vet ./...

# Validate every JSON file in every set-*/ directory against schema.json.
# Uses ajv-cli@5 + draft 2020-12 + strict=false (see configs/mock-data/README.md).
validate:
	cd $(ROOT) && npx -y -p ajv-cli@5 ajv validate \
		--spec=draft2020 --strict=false \
		-s configs/mock-data/schema.json \
		-d "configs/mock-data/set-*/*.json"

all: gen-small validate
