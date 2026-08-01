.PHONY: install start start-worker start-classic stop cc status logs

install:
	composer install --no-progress

cc:
	rm -rf var/cache/*
	rm -rf var/log/*

start: install
	docker compose up -d
	@echo "🧟 Igor Leak Labs are running!"
	@echo "👉 WORKER mode:  http://localhost:8080"
	@echo "👉 CLASSIC mode: http://localhost:8081"

start-worker: install
	docker compose up -d php-worker
	@echo "🧟 Igor Leak Lab in WORKER mode is running on http://localhost:8080"

start-classic: install
	docker compose up -d php-classic
	@echo "🧟 Igor Leak Lab in CLASSIC mode is running on http://localhost:8081"

stop:
	docker compose down

status:
	docker compose ps

logs:
	docker compose logs -f
