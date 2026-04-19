.PHONY: web-install web-build web-clean build clean dev

web-install:
	cd web && npm install

web-build: web-install
	cd web && npm run build
	rm -rf internal/server/web_dist
	cp -r web/dist internal/server/web_dist

web-clean:
	rm -rf web/dist web/node_modules internal/server/web_dist
	mkdir -p internal/server/web_dist
	touch internal/server/web_dist/.gitkeep

build: web-build
	go build -o ocp ./cmd/ocp

clean: web-clean
	rm -f ocp

dev:
	cd web && npm run dev
