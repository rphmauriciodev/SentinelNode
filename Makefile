SHELL := /bin/bash
.DEFAULT_GOAL := help

PROJECT_ROOT := $(CURDIR)
BIN_DIR      := $(PROJECT_ROOT)/bin
LOG_DIR      := $(PROJECT_ROOT)/logs
RUN_DIR      := $(PROJECT_ROOT)/.run
CERTS_DIR    := $(PROJECT_ROOT)/certs
VENV_DIR     := $(PROJECT_ROOT)/.venv
PYTHON       := $(VENV_DIR)/bin/python3
PIP          := $(VENV_DIR)/bin/pip

GREEN  := \033[0;32m
YELLOW := \033[1;33m
RED    := \033[0;31m
BLUE   := \033[0;34m
CYAN   := \033[0;36m
BOLD   := \033[1m
RESET  := \033[0m

.PHONY: help up start down stop restart dev status logs logs-gateway logs-engine logs-emulator logs-docker test build certs clean venv flash-pir flash-cam \
        docker-build docker-up docker-down docker-dev docker-logs docker-status \
        host-up host-down host-dev

help:
	@echo -e "$(BOLD)$(CYAN)========================================================================$(RESET)"
	@echo -e "$(BOLD)$(CYAN)                SENTINELNODE 2.0 - COMANDOS DE CONTROLE                 $(RESET)"
	@echo -e "$(BOLD)$(CYAN)========================================================================$(RESET)"
	@echo -e "  $(BOLD)$(GREEN)make up$(RESET) / $(BOLD)$(GREEN)make start$(RESET)    Inicializa toda a stack Docker (Gateway, Engine, Broker, DB)"
	@echo -e "  $(BOLD)$(GREEN)make dev$(RESET)               Inicializa a stack completa + Emulador de câmera via Docker"
	@echo -e "  $(BOLD)$(YELLOW)make stop$(RESET) / $(BOLD)$(YELLOW)make down$(RESET)  Encerra graciosamente todos os containers e serviços"
	@echo -e "  $(BOLD)$(YELLOW)make restart$(RESET)            Reinicia toda a stack do ecossistema"
	@echo -e "  $(BOLD)$(BLUE)make status$(RESET)             Exibe o status dos containers e processos"
	@echo -e "  $(BOLD)$(BLUE)make logs$(RESET)               Acompanha os logs em tempo real via Docker Compose"
	@echo -e "  $(BOLD)$(BLUE)make docker-build$(RESET)       Reconstrói todas as imagens Docker locais"
	@echo -e "  $(BOLD)$(BLUE)make test$(RESET)               Executa a suíte de testes de todos os módulos"
	@echo -e "  $(BOLD)$(CYAN)make certs$(RESET)              Gera/regenera certificados mTLS x509"
	@echo -e "  $(BOLD)$(CYAN)make host-up$(RESET)            Inicia serviços nativamente no host (sem Docker para Go)"
	@echo -e "  $(BOLD)$(RED)make clean$(RESET)              Encerra processos e remove binários, logs e PIDs"
	@echo -e "$(BOLD)$(CYAN)========================================================================$(RESET)"

certs:
	@echo -e "$(CYAN)🔑 Verificando certificados mTLS...$(RESET)"
	@if [ ! -f "$(CERTS_DIR)/ca.crt" ] || [ ! -f "$(CERTS_DIR)/server.crt" ] || [ ! -f "$(CERTS_DIR)/client.crt" ]; then \
		echo -e "$(YELLOW)Gerando novos certificados mTLS...$(RESET)"; \
		cd $(CERTS_DIR) && chmod +x generate_certs.sh && ./generate_certs.sh; \
	else \
		echo -e "$(GREEN)Certificados mTLS válidos em $(CERTS_DIR)!$(RESET)"; \
	fi

docker-build:
	@echo -e "$(CYAN)🔨 Construindo imagens Docker da stack SentinelNode...$(RESET)"
	@docker compose build

docker-up: certs
	@echo -e "$(CYAN)🐳 Subindo ecossistema SentinelNode 2.0 via Docker Compose...$(RESET)"
	@docker compose up -d
	@echo -e "\n$(BOLD)$(GREEN)========================================================================$(RESET)"
	@echo -e "$(BOLD)$(GREEN)   SENTINELNODE 2.0 DOCKER STACK ONLINE!                               $(RESET)"
	@echo -e "$(BOLD)$(GREEN)========================================================================$(RESET)"
	@echo -e "  🌐 $(BOLD)Interface Web de Vigilância:$(RESET)  http://localhost:8080/"
	@echo -e "  📹 $(BOLD)Live Stream Centralizado:$(RESET)     http://localhost:8080/live?device_id=esp32-cam-01"
	@echo -e "  📊 $(BOLD)Status da API Gateway:$(RESET)        http://localhost:8080/status"
	@echo -e "  🔒 $(BOLD)Broker MQTT mTLS:$(RESET)             localhost:8883"
	@echo -e "  💾 $(BOLD)PostgreSQL 15:$(RESET)                localhost:5435 (db: sentinel_db)"
	@echo -e "$(BOLD)$(GREEN)========================================================================$(RESET)\n"

docker-dev: certs
	@echo -e "$(CYAN)🐳 Subindo stack SentinelNode + Emulador de Câmera via Docker...$(RESET)"
	@docker compose --profile emulator up -d
	@echo -e "\n$(BOLD)$(GREEN)========================================================================$(RESET)"
	@echo -e "$(BOLD)$(GREEN)   SENTINELNODE 2.0 + EMULADOR ATIVOS NO DOCKER!                       $(RESET)"
	@echo -e "$(BOLD)$(GREEN)========================================================================$(RESET)"
	@echo -e "  🌐 $(BOLD)Interface Web de Vigilância:$(RESET)  http://localhost:8080/"
	@echo -e "  🎥 $(BOLD)Emulador de Câmera (:8087):$(RESET)   http://localhost:8087/stream"
	@echo -e "  🚨 $(BOLD)Disparar Evento PIR (Teste):$(RESET)  curl -X POST http://localhost:8087/pir/trigger"
	@echo -e "$(BOLD)$(GREEN)========================================================================$(RESET)\n"

docker-down:
	@echo -e "$(YELLOW)Parando todos os containers Docker...$(RESET)"
	@docker compose --profile emulator down
	@echo -e "$(GREEN)Containers Docker encerrados com sucesso.$(RESET)"

docker-logs:
	@docker compose logs -f

docker-status:
	@docker compose ps

docker-infra: certs
	@echo -e "$(CYAN)🐳 Subindo infraestrutura básica (Mosquitto & PostgreSQL)...$(RESET)"
	@docker compose up -d broker db
	@echo -e "$(YELLOW)Aguardando PostgreSQL e Mosquitto ficarem prontos...$(RESET)"
	@timeout 20 bash -c 'until (echo > /dev/tcp/localhost/5435) >/dev/null 2>&1; do sleep 0.5; done' || (echo -e "$(RED)Timeout aguardando PostgreSQL na porta 5435!$(RESET)" && exit 1)
	@timeout 20 bash -c 'until (echo > /dev/tcp/localhost/1883) >/dev/null 2>&1; do sleep 0.5; done' || (echo -e "$(RED)Timeout aguardando Mosquitto na porta 1883!$(RESET)" && exit 1)
	@echo -e "$(GREEN)Infraestrutura Docker online e pronta!$(RESET)"

build:
	@echo -e "$(CYAN)🔨 Compilando binários Go...$(RESET)"
	@mkdir -p $(BIN_DIR)
	@cd $(PROJECT_ROOT)/media-gateway && go build -o $(BIN_DIR)/media-gateway .
	@cd $(PROJECT_ROOT)/ingestion-engine && go build -o $(BIN_DIR)/ingestion-engine ./cmd/main.go
	@echo -e "$(GREEN)Binários compilados em $(BIN_DIR)!$(RESET)"

start-gateway: build
	@mkdir -p $(LOG_DIR) $(RUN_DIR)
	@if [ -f "$(RUN_DIR)/media-gateway.pid" ] && kill -0 $$(cat "$(RUN_DIR)/media-gateway.pid") 2>/dev/null; then \
		echo -e "$(YELLOW)Media Gateway já em execução (PID: $$(cat $(RUN_DIR)/media-gateway.pid))$(RESET)"; \
	else \
		fuser -k 8080/tcp 2>/dev/null || true; \
		echo -e "$(CYAN)Iniciando Media Gateway (:8080)...$(RESET)"; \
		sh -c 'cd $(PROJECT_ROOT)/media-gateway && exec $(BIN_DIR)/media-gateway > $(LOG_DIR)/media-gateway.log 2>&1 & echo $$! > $(RUN_DIR)/media-gateway.pid'; \
		sleep 1; \
		if kill -0 $$(cat "$(RUN_DIR)/media-gateway.pid") 2>/dev/null; then \
			echo -e "$(GREEN)Media Gateway ativo! PID: $$(cat $(RUN_DIR)/media-gateway.pid)$(RESET)"; \
		else \
			echo -e "$(RED)Falha ao iniciar Media Gateway! Verifique $(LOG_DIR)/media-gateway.log$(RESET)"; \
			cat $(LOG_DIR)/media-gateway.log; \
			exit 1; \
		fi; \
	fi

start-engine: build
	@mkdir -p $(LOG_DIR) $(RUN_DIR)
	@if [ -f "$(RUN_DIR)/ingestion-engine.pid" ] && kill -0 $$(cat "$(RUN_DIR)/ingestion-engine.pid") 2>/dev/null; then \
		echo -e "$(YELLOW)Ingestion Engine já em execução (PID: $$(cat $(RUN_DIR)/ingestion-engine.pid))$(RESET)"; \
	else \
		echo -e "$(CYAN)Iniciando Ingestion Engine...$(RESET)"; \
		sh -c 'cd $(PROJECT_ROOT)/ingestion-engine && exec $(BIN_DIR)/ingestion-engine > $(LOG_DIR)/ingestion-engine.log 2>&1 & echo $$! > $(RUN_DIR)/ingestion-engine.pid'; \
		sleep 1; \
		if kill -0 $$(cat "$(RUN_DIR)/ingestion-engine.pid") 2>/dev/null; then \
			echo -e "$(GREEN)Ingestion Engine ativo! PID: $$(cat $(RUN_DIR)/ingestion-engine.pid)$(RESET)"; \
		else \
			echo -e "$(RED)Falha ao iniciar Ingestion Engine! Verifique $(LOG_DIR)/ingestion-engine.log$(RESET)"; \
			cat $(LOG_DIR)/ingestion-engine.log; \
			exit 1; \
		fi; \
	fi

venv:
	@if [ ! -f "$(PYTHON)" ]; then \
		echo -e "$(CYAN)Criando ambiente virtual Python em $(VENV_DIR)...$(RESET)"; \
		python3 -m venv $(VENV_DIR); \
		echo -e "$(CYAN)Instalando dependências do emulador...$(RESET)"; \
		$(PIP) install -q -r $(PROJECT_ROOT)/firmware/ESP-CAM/requirements.txt; \
		echo -e "$(GREEN)Ambiente Python pronto!$(RESET)"; \
	fi

start-emulator: venv
	@mkdir -p $(LOG_DIR) $(RUN_DIR)
	@if [ -f "$(RUN_DIR)/emulator.pid" ] && kill -0 $$(cat "$(RUN_DIR)/emulator.pid") 2>/dev/null; then \
		echo -e "$(YELLOW)Emulador já em execução (PID: $$(cat $(RUN_DIR)/emulator.pid))$(RESET)"; \
	else \
		fuser -k 8087/tcp 2>/dev/null || true; \
		echo -e "$(CYAN)Iniciando Emulador de Câmera (:8087)...$(RESET)"; \
		sh -c 'cd $(PROJECT_ROOT)/firmware/ESP-CAM && exec $(PYTHON) emulator.py > $(LOG_DIR)/emulator.log 2>&1 & echo $$! > $(RUN_DIR)/emulator.pid'; \
		sleep 1; \
		if kill -0 $$(cat "$(RUN_DIR)/emulator.pid") 2>/dev/null; then \
			echo -e "$(GREEN)Emulador de Câmera ativo! PID: $$(cat $(RUN_DIR)/emulator.pid)$(RESET)"; \
		else \
			echo -e "$(RED)Falha ao iniciar Emulador! Verifique $(LOG_DIR)/emulator.log$(RESET)"; \
			cat $(LOG_DIR)/emulator.log; \
			exit 1; \
		fi; \
	fi

up: docker-up
start: up

dev: docker-dev

host-up: docker-infra start-gateway start-engine
	@echo -e "\n$(BOLD)$(GREEN)========================================================================$(RESET)"
	@echo -e "$(BOLD)$(GREEN)   SENTINELNODE 2.0 (HOST NATIVO) INICIALIZADO COM SUCESSO!            $(RESET)"
	@echo -e "$(BOLD)$(GREEN)========================================================================$(RESET)"
	@echo -e "  🌐 $(BOLD)Interface Web de Vigilância:$(RESET)  http://localhost:8080/"
	@echo -e "  📹 $(BOLD)Live Stream Centralizado:$(RESET)     http://localhost:8080/live?device_id=esp32-cam-01"
	@echo -e "  📊 $(BOLD)Status da API Gateway:$(RESET)        http://localhost:8080/status"
	@echo -e "  🔒 $(BOLD)Broker MQTT mTLS:$(RESET)             localhost:8883"
	@echo -e "  💾 $(BOLD)PostgreSQL 15:$(RESET)                localhost:5435 (db: sentinel_db)"
	@echo -e "$(BOLD)$(GREEN)========================================================================$(RESET)\n"

host-dev: host-up start-emulator
host-down: stop-emulator stop-engine stop-gateway
stop-emulator:
	@if [ -f "$(RUN_DIR)/emulator.pid" ]; then \
		PID=$$(cat "$(RUN_DIR)/emulator.pid"); \
		if kill -0 $$PID 2>/dev/null; then \
			echo -e "$(YELLOW)Encerrando Emulador de Câmera (PID: $$PID)...$(RESET)"; \
			kill -15 $$PID 2>/dev/null || true; \
		fi; \
		rm -f "$(RUN_DIR)/emulator.pid"; \
	fi
	@fuser -k 8087/tcp 2>/dev/null || true
	@sleep 1

stop-engine:
	@if [ -f "$(RUN_DIR)/ingestion-engine.pid" ]; then \
		PID=$$(cat "$(RUN_DIR)/ingestion-engine.pid"); \
		if kill -0 $$PID 2>/dev/null; then \
			echo -e "$(YELLOW)Encerrando Ingestion Engine graciosamente (PID: $$PID)...$(RESET)"; \
			kill -2 $$PID 2>/dev/null || true; \
			for i in $$(seq 1 10); do \
				kill -0 $$PID 2>/dev/null || break; \
				sleep 0.5; \
			done; \
			if kill -0 $$PID 2>/dev/null; then \
				kill -9 $$PID 2>/dev/null || true; \
			fi; \
		fi; \
		rm -f "$(RUN_DIR)/ingestion-engine.pid"; \
	fi

stop-gateway:
	@if [ -f "$(RUN_DIR)/media-gateway.pid" ]; then \
		PID=$$(cat "$(RUN_DIR)/media-gateway.pid"); \
		if kill -0 $$PID 2>/dev/null; then \
			echo -e "$(YELLOW)Encerrando Media Gateway graciosamente (PID: $$PID)...$(RESET)"; \
			kill -2 $$PID 2>/dev/null || true; \
			for i in $$(seq 1 10); do \
				kill -0 $$PID 2>/dev/null || break; \
				sleep 0.5; \
			done; \
			if kill -0 $$PID 2>/dev/null; then \
				echo -e "$(RED)Forçando encerramento do Media Gateway...$(RESET)"; \
				kill -9 $$PID 2>/dev/null || true; \
			fi; \
		fi; \
		rm -f "$(RUN_DIR)/media-gateway.pid"; \
	fi
	@fuser -k 8080/tcp 2>/dev/null || true
down: stop-emulator stop-engine stop-gateway docker-down
stop: down

restart: stop start

status:
	@echo -e "$(BOLD)$(CYAN)================== STATUS DOS CONTAINERS DOCKER ==================$(RESET)"
	@docker compose ps
	@echo -e "$(BOLD)$(CYAN)=================== STATUS DOS PROCESSOS NATIVOS ===================$(RESET)"
	@if [ -f "$(RUN_DIR)/media-gateway.pid" ] && kill -0 $$(cat "$(RUN_DIR)/media-gateway.pid") 2>/dev/null; then \
		echo -e "  Media Gateway (:8080):     $(GREEN)ATIVO$(RESET) (PID: $$(cat $(RUN_DIR)/media-gateway.pid))"; \
	else \
		echo -e "  Media Gateway (:8080):     $(RED)PARADO$(RESET)"; \
	fi
	@if [ -f "$(RUN_DIR)/ingestion-engine.pid" ] && kill -0 $$(cat "$(RUN_DIR)/ingestion-engine.pid") 2>/dev/null; then \
		echo -e "  Ingestion Engine:          $(GREEN)ATIVO$(RESET) (PID: $$(cat $(RUN_DIR)/ingestion-engine.pid))"; \
	else \
		echo -e "  Ingestion Engine:          $(RED)PARADO$(RESET)"; \
	fi
	@if [ -f "$(RUN_DIR)/emulator.pid" ] && kill -0 $$(cat "$(RUN_DIR)/emulator.pid") 2>/dev/null; then \
		echo -e "  Emulador Câmera (:8087):   $(GREEN)ATIVO$(RESET) (PID: $$(cat $(RUN_DIR)/emulator.pid))"; \
	else \
		echo -e "  Emulador Câmera (:8087):   $(RED)PARADO$(RESET)"; \
	fi
	@echo -e "$(BOLD)$(CYAN)=====================================================================$(RESET)"

logs: docker-logs

logs-gateway:
	@tail -f $(LOG_DIR)/media-gateway.log

logs-engine:
	@tail -f $(LOG_DIR)/ingestion-engine.log

logs-emulator:
	@tail -f $(LOG_DIR)/emulator.log

logs-docker:
	@docker compose logs -f

test:
	@echo -e "$(BOLD)$(CYAN)🧪 Executando suíte de testes de todos os módulos...$(RESET)"
	@echo -e "$(BLUE)1. Testes do Media Gateway...$(RESET)"
	@cd $(PROJECT_ROOT)/media-gateway && go test -v ./...
	@echo -e "$(BLUE)2. Testes do Ingestion Engine...$(RESET)"
	@cd $(PROJECT_ROOT)/ingestion-engine && go test -v ./...
	@echo -e "$(BLUE)3. Testes do Edge Agent...$(RESET)"
	@cd $(PROJECT_ROOT)/edge-agent && go test -v ./...
	@echo -e "$(BLUE)4. Testes do Protocolo Binário C++...$(RESET)"
	@cd $(PROJECT_ROOT)/firmware/test && ./test_protocol
	@echo -e "$(BOLD)$(GREEN)🎉 Todos os testes passaram com sucesso!$(RESET)"

clean: stop
	@echo -e "$(YELLOW)🧹 Limpando arquivos temporários, logs e binários...$(RESET)"
	@rm -rf $(BIN_DIR) $(LOG_DIR) $(RUN_DIR)
	@echo -e "$(GREEN)Limpeza concluída!$(RESET)"

flash-pir:
	@echo -e "$(CYAN)⚡ Gravando firmware ESP-PIR na porta serial...$(RESET)"
	@cd $(PROJECT_ROOT)/firmware/ESP-PIR && pio run -t upload
	@echo -e "$(GREEN)Firmware ESP-PIR gravado com sucesso!$(RESET)"

flash-cam:
	@echo -e "$(CYAN)⚡ Gravando firmware ESP32-CAM na porta serial...$(RESET)"
	@cd $(PROJECT_ROOT)/firmware && pio run -t upload
	@echo -e "$(GREEN)Firmware ESP32-CAM gravado com sucesso!$(RESET)"
