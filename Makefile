.PHONY: install test bundle
install:
	cd webapp && npm ci
test:
	go test -race ./server
	cd webapp && npm test && npm run build && npm run test:browser
bundle:
	bash scripts/package.sh
