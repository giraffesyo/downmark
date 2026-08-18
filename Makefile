VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GOROOT_DIR := $(shell go env GOROOT)

# Features the Go toolchain emits for js/wasm. These must be named
# explicitly: which ones a binaryen build enables by default varies by
# version, and older releases (e.g. the one in Ubuntu's apt) reject
# i64.extend32_s without --enable-sign-ext.
WASM_OPT_FEATURES := --enable-sign-ext --enable-bulk-memory --enable-nontrapping-float-to-int

.PHONY: wasm-exec wasm js js-test js-test-browser

# Copy the Go runtime's JS support shim into the npm package's vendor dir.
# Go 1.24+ ships it under lib/wasm (previously misc/wasm).
# install -m instead of cp: the toolchain file is mode 0444 and cp would
# propagate that, breaking the next copy.
wasm-exec:
	install -m 0644 "$(GOROOT_DIR)/lib/wasm/wasm_exec.js" js/vendor/wasm_exec.js

wasm: wasm-exec
	mkdir -p js/dist
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o js/dist/downmark.wasm ./wasm
	@if command -v wasm-opt >/dev/null 2>&1; then \
		echo "wasm-opt -Oz js/dist/downmark.wasm"; \
		wasm-opt -Oz $(WASM_OPT_FEATURES) -o js/dist/downmark.wasm.opt js/dist/downmark.wasm && \
		mv js/dist/downmark.wasm.opt js/dist/downmark.wasm; \
	fi
	@ls -lh js/dist/downmark.wasm

js: wasm
	cd js && npm ci && npm run build:ts

js-test: js
	cd js && npm test

# Separate from js-test because it needs a browser binary:
# run `cd js && npx playwright install chromium` once first.
js-test-browser: js
	cd js && npm run test:browser
