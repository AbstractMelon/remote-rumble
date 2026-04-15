FRONTEND_DIR=frontend
BACKEND_WEB_DIR=backend/web

.PHONY: dev-backend dev-frontend build docker-up docker-down

dev-backend:
	go run ./backend

dev-frontend:
	cd $(FRONTEND_DIR) && npm run dev

build:
	cd $(FRONTEND_DIR) && npm install && npm run build
	rm -rf $(BACKEND_WEB_DIR)
	mkdir -p $(BACKEND_WEB_DIR)
	cp -r $(FRONTEND_DIR)/build/* $(BACKEND_WEB_DIR)/
	go build -o remote-rumble ./backend

docker-up:
	docker compose up -d

docker-down:
	docker compose down
