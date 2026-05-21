# O-Cloud Edge Platform — Root Makefile
# 顶层快捷命令，转发到各模块。每个模块仍维护自己的 Makefile。

SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

# 颜色
CYAN  := \033[36m
GREEN := \033[32m
YELLOW:= \033[33m
RED   := \033[31m
NC    := \033[0m

# =========================================
# 帮助
# =========================================
.PHONY: help
help: ## 显示可用目标
	@printf "$(CYAN)O-Cloud Edge Platform — Root Makefile$(NC)\n\n"
	@printf "$(GREEN)用法:$(NC) make $(YELLOW)<target>$(NC)\n\n"
	@printf "$(GREEN)模块:$(NC)\n"
	@printf "  backend       — Go 演示后端\n"
	@printf "  frontend      — React 演示前端\n"
	@printf "  operators     — Kubebuilder CRDs\n"
	@printf "  configs       — Mock 数据与配置\n"
	@printf "  deploy        — 部署与 CI\n\n"
	@printf "$(GREEN)可用目标:$(NC)\n"
	@grep -E '^[a-zA-Z_/-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(YELLOW)%-22s$(NC) %s\n", $$1, $$2}'

# =========================================
# 模块快捷命令
# =========================================
.PHONY: backend/build backend/test backend/lint backend/run
backend/build: ## backend: 编译
	@$(MAKE) -C backend build
backend/test: ## backend: 单测
	@$(MAKE) -C backend test
backend/lint: ## backend: lint
	@$(MAKE) -C backend lint
backend/run: ## backend: 本地启动
	@$(MAKE) -C backend run

.PHONY: frontend/build frontend/test frontend/lint frontend/dev
frontend/build: ## frontend: 生产构建
	@cd frontend && pnpm run build
frontend/test: ## frontend: 单测
	@cd frontend && pnpm run test
frontend/lint: ## frontend: lint + typecheck
	@cd frontend && pnpm run lint && pnpm run typecheck
frontend/dev: ## frontend: 启动 dev server
	@cd frontend && pnpm run dev

.PHONY: operators/generate operators/manifests operators/test
operators/generate: ## operators: 生成 deepcopy
	@$(MAKE) -C operators/pool-operator generate
operators/manifests: ## operators: 生成 CRD YAML
	@$(MAKE) -C operators/pool-operator manifests
operators/test: ## operators: 单测
	@$(MAKE) -C operators/pool-operator test

.PHONY: o2-dms-adapter/build o2-dms-adapter/test o2-dms-adapter/lint o2-dms-adapter/image
o2-dms-adapter/build: ## o2-dms-adapter: 编译 (Phase 9 P9-T-008 scaffold)
	@$(MAKE) -C operators/o2-dms-adapter build
o2-dms-adapter/test: ## o2-dms-adapter: 单测 (7 stub-routing + 1 404 cases)
	@$(MAKE) -C operators/o2-dms-adapter test
o2-dms-adapter/lint: ## o2-dms-adapter: vet
	@$(MAKE) -C operators/o2-dms-adapter lint
o2-dms-adapter/image: ## o2-dms-adapter: docker build
	@$(MAKE) -C operators/o2-dms-adapter image

.PHONY: configs/validate configs/gen-mock
configs/validate: ## configs: 校验 mock 数据
	@$(MAKE) -C configs/mock-data validate
configs/gen-mock: ## configs: 重新生成所有 mock 数据集
	@$(MAKE) -C configs/mock-data all

.PHONY: dev-up dev-down dev-logs
dev-up: ## 本地开发栈启动（docker-compose）
	@docker-compose -f deploy/dev/docker-compose.yaml up -d
dev-down: ## 本地开发栈停止
	@docker-compose -f deploy/dev/docker-compose.yaml down
dev-logs: ## 本地开发栈日志
	@docker-compose -f deploy/dev/docker-compose.yaml logs -f

# =========================================
# 一键流水线
# =========================================
.PHONY: ci ci-fast lint test build
ci: lint test build ## 完整 CI（lint + test + build）
ci-fast: lint ## 快速检查（只 lint）

lint: ## 全仓库 lint
	@$(MAKE) backend/lint
	@$(MAKE) frontend/lint
	@$(MAKE) configs/validate

test: ## 全仓库 test
	@$(MAKE) backend/test
	@$(MAKE) frontend/test
	@$(MAKE) operators/test

build: ## 全仓库构建
	@$(MAKE) backend/build
	@$(MAKE) frontend/build
	@$(MAKE) operators/generate operators/manifests

# =========================================
# 契约校验
# =========================================
.PHONY: contract-validate contract-diff
contract-validate: ## 校验 OpenAPI 契约
	@command -v swagger-cli >/dev/null || { echo "$(RED)swagger-cli 未安装$(NC) (npm i -g @apidevtools/swagger-cli)"; exit 1; }
	@swagger-cli validate docs/api-contract.yaml

contract-diff: ## 显示契约与上游 dev 分支的差异
	@git diff origin/dev -- docs/api-contract.yaml

# =========================================
# 清理
# =========================================
.PHONY: clean clean-all
clean: ## 清理各模块构建产物
	@$(MAKE) -C backend clean 2>/dev/null || true
	@rm -rf frontend/dist frontend/.vite
	@rm -rf operators/pool-operator/bin
	@find . -type f -name "*.log" -delete

clean-all: clean ## 清理 + 删除依赖
	@rm -rf frontend/node_modules
	@rm -rf backend/vendor
