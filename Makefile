.PHONY: test build new-rule new-handler scenarios-analysis ui-install ui-dev ui-build ui-test ui-test-e2e

test:
	go test ./...

build:
	go build ./...

new-rule:
ifndef NAME
	$(error NAME is required, e.g. make new-rule NAME=my_rule)
endif
	./tools/scaffold-rule.sh $(NAME)

new-handler:
ifndef NAME
	$(error NAME is required, e.g. make new-handler NAME=my_kind)
endif
	./tools/scaffold-handler.sh $(NAME)

scenarios-analysis:
	@echo "Run from the gno-cluster repo: .ignore/scripts/scenarios/run-analysis-scenarios.sh"

ui-install:
	cd internal/scribe/ui/frontend && npm install

ui-dev:
	cd internal/scribe/ui/frontend && npm run dev

ui-build:
	cd internal/scribe/ui/frontend && npm run build

ui-test:
	cd internal/scribe/ui/frontend && npm run test

ui-test-e2e:
	cd internal/scribe/ui/frontend && npm run test:e2e
