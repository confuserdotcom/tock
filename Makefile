VERSION ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)

# Packaging and TCC identity (plan 01-08). The bundle path and bundle id are
# what Accessibility and Automation grants key on; never change them.
TOCK_DATA ?= $(HOME)/.local/share/tock
TOCK_STATE ?= $(HOME)/.local/state/tock
BUNDLE ?= $(TOCK_DATA)/trackd.app
BUNDLE_ID := com.tock.trackd
AGENT_LABEL := com.tock.trackd
SIGN_ID ?= tock-local-codesign
# auto | cert | adhoc
SIGN_MODE ?= auto
CERT_PEM ?= $(TOCK_DATA)/codesign/tock-local-codesign.pem
CODESIGN_TIMEOUT ?= 60
# macOS has no GNU timeout. A perl alarm survives exec, so the wrapped command
# receives SIGALRM and exits 142 when it overruns.
WITH_TIMEOUT = perl -e 'alarm shift; exec @ARGV or exit 127'

.PHONY: build test test-quick vet clean bundle print-dr

build:
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" -o bin/trackd ./cmd/trackd

test-quick:
	CGO_ENABLED=0 go test ./internal/span/... ./internal/store/... -count=1

test:
	go vet ./...
	go build ./...
	CGO_ENABLED=0 go test ./internal/span/... ./internal/store/... -count=1
	go test ./... -count=1

vet:
	go vet ./...

clean:
	rm -rf bin build

# bundle: build trackd with cgo, assemble the .app, embed Info.plist, sign.
bundle:
	@if launchctl print "gui/$$(id -u)/$(AGENT_LABEL)" >/dev/null 2>&1; then \
		echo "agent $(AGENT_LABEL) is loaded; use make install" >&2; exit 1; \
	fi
	@mkdir -p build
	CGO_ENABLED=1 go build \
		-ldflags "-X main.version=$(VERSION) -extldflags=-Wl,-sectcreate,__TEXT,__info_plist,$(CURDIR)/packaging/Info.plist" \
		-o build/trackd ./cmd/trackd
	@mkdir -p "$(TOCK_DATA)" && chmod 700 "$(TOCK_DATA)"
	@mkdir -p "$(BUNDLE)/Contents/MacOS"
	cp build/trackd "$(BUNDLE)/Contents/MacOS/trackd"
	cp packaging/Info.plist "$(BUNDLE)/Contents/Info.plist"
	chmod -R go-w "$(BUNDLE)"
	@mode="$(SIGN_MODE)"; \
	have_id=0; \
	if security find-identity -v -p codesigning 2>/dev/null | grep -q '"$(SIGN_ID)"'; then have_id=1; fi; \
	case "$$mode" in \
		auto) \
			if [ "$$have_id" = 1 ]; then mode=cert; else \
				echo "WARNING: signing ad-hoc (identifier-only requirement); run make cert && make cert-trust for a certificate-anchored identity" >&2; \
				mode=adhoc; \
			fi ;; \
		cert) \
			if [ "$$have_id" != 1 ]; then \
				echo "signing identity $(SIGN_ID) not found or not trusted; run make cert && make cert-trust, or SIGN_MODE=adhoc" >&2; \
				exit 1; \
			fi ;; \
		adhoc) ;; \
		*) echo "unknown SIGN_MODE '$$mode' (auto|cert|adhoc)" >&2; exit 1 ;; \
	esac; \
	echo "bundle: signing mode $$mode"; \
	rc=0; \
	if [ "$$mode" = cert ]; then \
		$(WITH_TIMEOUT) $(CODESIGN_TIMEOUT) codesign --force --sign "$(SIGN_ID)" --identifier $(BUNDLE_ID) "$(BUNDLE)" || rc=$$?; \
	else \
		$(WITH_TIMEOUT) $(CODESIGN_TIMEOUT) codesign --force --sign - --identifier $(BUNDLE_ID) -r='designated => identifier "com.tock.trackd"' "$(BUNDLE)" || rc=$$?; \
	fi; \
	if [ "$$rc" = 142 ]; then \
		echo "codesign timed out after $(CODESIGN_TIMEOUT) s: a keychain prompt for the $(SIGN_ID) key is probably waiting; see 'Key access' in docs/spikes/01-identity-lock-spike.md (set-key-partition-list or Always Allow)" >&2; \
		exit 1; \
	fi; \
	[ "$$rc" = 0 ] || exit $$rc
	codesign --verify --strict --verbose=2 "$(BUNDLE)"
	codesign -d -r- "$(BUNDLE)"

print-dr:
	@codesign -d -r- "$(BUNDLE)" 2>&1 | grep -E '^designated|# designated' || true
	@codesign -dv "$(BUNDLE)" 2>&1 | grep -E '^CDHash' || true
