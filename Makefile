.PHONY: migrate-create migrate-up migrate-down

MIGRATIONS_DIR := migrations
DATABASE_URL ?= $(shell yq -r '.database_url' env.yaml)

.PHONY: migrate-create
migrate-create:
	@test -n "$(name)" || (echo "Usage: make migrate-create name=create_users_table"; exit 1)
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(name)

.PHONY: migrate-up
migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down

.PHONY: migrate-force
migrate-force:
	@test -n "$(version)" || (echo "Usage: make migrate-force version=1"; exit 1)
	migrate -path migrations \
      -database $(DATABASE_URL) \
      force $(version)