.PHONY: up down restart logs add-validator remove-validator

COMPOSE = docker compose -f docker-compose.yml --env-file config.env

up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

restart:
	$(COMPOSE) restart $(svc)

logs:
	$(COMPOSE) logs -f $(svc)

# ---- Validator management

# Usage: make add-validator name=val-01 permissions=rpc,metrics,logs,otlp logs_min_level=info
add-validator:
	@if [ -z "$(name)" ]; then echo "ERROR: name is required"; exit 1; fi
	@if [ -z "$(permissions)" ]; then echo "ERROR: permissions is required"; exit 1; fi
	@if [ -z "$(logs_min_level)" ]; then echo "ERROR: logs_min_level is required"; exit 1; fi
	@TOKEN=$$(openssl rand -hex 32); \
	printf '\n[validators.%s]\ntoken          = "%s"\npermissions    = [%s]\nlogs_min_level = "%s"\n' \
		"$(name)" \
		"$$TOKEN" \
		"$(shell echo "$(permissions)" | sed 's/,/", "/g; s/^/"/; s/$$/"/)" \
		"$(logs_min_level)" \
		>> watchtower.toml; \
	$(COMPOSE) restart watchtower; \
	echo ""; \
	echo "Validator $(name) added. Bearer token (paste into sentinel config):"; \
	echo "$$TOKEN"

# Usage: make remove-validator name=val-01
remove-validator:
	@if [ -z "$(name)" ]; then echo "ERROR: name is required"; exit 1; fi
	@python3 -c "import re; f=open('watchtower.toml','r'); c=f.read(); f.close(); u=re.sub(r'\n\[validators\.$(name)\].*?(?=\n\[|\Z)','',c,flags=re.S|re.M); f=open('watchtower.toml','w'); f.write(u); f.close(); print('Validator $(name) removed from watchtower.toml')"
	@$(COMPOSE) restart watchtower
