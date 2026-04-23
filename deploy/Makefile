.PHONY: help up down restart logs version show-validators add-validator remove-validator

COMPOSE = docker compose -f docker-compose.yml

.DEFAULT_GOAL := help

help:  ## Show this help
	@awk 'BEGIN{FS=":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

up:  ## Start the monitoring stack (docker compose up -d)
	@export COMMIT=$$(git -C .. rev-parse HEAD 2>/dev/null || echo "") \
	 && export BUILD_TIME=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
	 && $(COMPOSE) up -d --build

down:  ## Stop the stack
	$(COMPOSE) down

restart:  ## Restart a service (usage: make restart svc=watchtower)
	$(COMPOSE) restart $(svc)

logs:  ## Tail a service's logs (usage: make logs svc=watchtower)
	$(COMPOSE) logs -f $(svc)

version:  ## Print the watchtower build version (verbose)
	@$(COMPOSE) exec -T watchtower watchtower version -v

show-validators:  ## List validators configured in watchtower.toml (name + permissions, no tokens)
	@awk '\
/^\[validators\./ { name=$$0; sub(/^\[validators\./,"",name); sub(/\].*$$/,"",name); print "- " name; next } \
/^permissions[[:space:]]*=/ && name { print "  " $$0; next } \
/^logs_min_level[[:space:]]*=/ && name { print "  " $$0; next } \
/^\[/ && !/^\[validators\./ { name="" } \
' watchtower.toml

# ---- Validator management

# Usage: make add-validator name=val-01 permissions=rpc,metrics,logs,otlp logs_min_level=info
add-validator:  ## Add a validator (usage: make add-validator name=val-01 permissions=rpc,metrics,logs,otlp logs_min_level=info)
	@if [ -z "$(name)" ]; then echo "ERROR: name is required"; exit 1; fi
	@if [ -z "$(permissions)" ]; then echo "ERROR: permissions is required"; exit 1; fi
	@if [ -z "$(logs_min_level)" ]; then echo "ERROR: logs_min_level is required"; exit 1; fi
	@TOKEN=$$(openssl rand -hex 32); \
	PERMS=$$(echo "$(permissions)" | sed 's/,/", "/g; s/^/"/; s/$$/"/'); \
	printf '\n[validators.%s]\ntoken          = "%s"\npermissions    = [%s]\nlogs_min_level = "%s"\n' \
		"$(name)" \
		"$$TOKEN" \
		"$$PERMS" \
		"$(logs_min_level)" \
		>> watchtower.toml; \
	$(COMPOSE) restart watchtower; \
	echo ""; \
	echo "Validator $(name) added. Bearer token (paste into sentinel config):"; \
	echo "$$TOKEN"

# Usage: make remove-validator name=val-01
remove-validator:  ## Remove a validator (usage: make remove-validator name=val-01)
	@if [ -z "$(name)" ]; then echo "ERROR: name is required"; exit 1; fi
	@python3 -c "import re; f=open('watchtower.toml','r'); c=f.read(); f.close(); u=re.sub(r'\n\[validators\.$(name)\].*?(?=\n\[|\Z)','',c,flags=re.S|re.M); f=open('watchtower.toml','w'); f.write(u); f.close(); print('Validator $(name) removed from watchtower.toml')"
	@$(COMPOSE) restart watchtower
