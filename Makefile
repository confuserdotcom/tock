VERSION ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)

# Packaging and TCC identity (plan 01-08). The bundle path and bundle id are
# what Accessibility and Automation grants key on; never change them.
TOCK_DATA ?= $(HOME)/.local/share/tock
TOCK_STATE ?= $(HOME)/.local/state/tock
BUNDLE ?= $(TOCK_DATA)/trackd.app
BUNDLE_ID := com.tock.trackd
AGENT_LABEL := com.tock.trackd
# The menu bar app (v2 NTF-01/BAR-01): its own bundle, so notifications show
# as tock and trackd's TCC identity never changes.
BAR_BUNDLE ?= $(TOCK_DATA)/tock.app
BAR_ID := com.tock.bar
BIN_DIR ?= $(HOME)/.local/bin
SIGN_ID ?= tock-local-codesign
# auto | cert | adhoc. Pinned to cert (plan 01-09): a missing identity is a hard
# error, never a silent switch to ad-hoc that would orphan TCC grants.
SIGN_MODE ?= cert
CERT_PEM ?= $(TOCK_DATA)/codesign/tock-local-codesign.pem
CODESIGN_TIMEOUT ?= 60
# macOS has no GNU timeout. A perl alarm survives exec, so the wrapped command
# receives SIGALRM and exits 142 when it overruns.
WITH_TIMEOUT = perl -e 'alarm shift; exec @ARGV or exit 127'

.PHONY: install-linux uninstall-linux build test test-quick vet clean bundle bundle-bar print-dr cert cert-trust dump dump-bg dump-stop install uninstall

build:
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" -o bin/trackd ./cmd/trackd
	CGO_ENABLED=0 go build -o bin/tock ./cmd/tock
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" -o bin/tockbar ./cmd/tockbar

test-quick:
	CGO_ENABLED=0 go test ./internal/span/... ./internal/store/... ./internal/classify/... ./internal/stats/... ./internal/focus/... ./internal/config/... ./internal/report/... ./internal/suggest/... ./internal/claude/... ./internal/app/... ./internal/tui/... ./internal/bar/... ./internal/service/... -count=1

test:
	go vet ./...
	go build ./...
	CGO_ENABLED=0 go test ./internal/span/... ./internal/store/... ./internal/classify/... ./internal/stats/... ./internal/focus/... ./internal/config/... ./internal/report/... ./internal/suggest/... ./internal/claude/... ./internal/app/... ./internal/tui/... ./internal/bar/... ./internal/service/... -count=1
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

# bundle-bar: build the menu bar app, assemble tock.app and sign it with the
# same identity as trackd (ad-hoc only when SIGN_MODE=adhoc, or auto without
# the identity). Notification access keys on the bundle id.
bundle-bar:
	@if launchctl print "gui/$$(id -u)/$(BAR_ID)" >/dev/null 2>&1; then \
		echo "agent $(BAR_ID) is loaded; use make install" >&2; exit 1; \
	fi
	@mkdir -p build
	CGO_ENABLED=1 go build -ldflags "-X main.version=$(VERSION)" -o build/tockbar ./cmd/tockbar
	@mkdir -p "$(TOCK_DATA)" && chmod 700 "$(TOCK_DATA)"
	@mkdir -p "$(BAR_BUNDLE)/Contents/MacOS"
	cp build/tockbar "$(BAR_BUNDLE)/Contents/MacOS/tockbar"
	cp packaging/bar/Info.plist "$(BAR_BUNDLE)/Contents/Info.plist"
	chmod -R go-w "$(BAR_BUNDLE)"
	@id="$(SIGN_ID)"; \
	if [ "$(SIGN_MODE)" = adhoc ] || ! security find-identity -v -p codesigning 2>/dev/null | grep -q '"$(SIGN_ID)"'; then \
		[ "$(SIGN_MODE)" = cert ] && { echo "signing identity $(SIGN_ID) not found; run make cert && make cert-trust, or SIGN_MODE=adhoc" >&2; exit 1; }; \
		id=-; \
	fi; \
	rc=0; $(WITH_TIMEOUT) $(CODESIGN_TIMEOUT) codesign --force --sign "$$id" --identifier $(BAR_ID) "$(BAR_BUNDLE)" || rc=$$?; \
	if [ "$$rc" = 142 ]; then echo "codesign timed out after $(CODESIGN_TIMEOUT) s: a keychain prompt is probably waiting" >&2; exit 1; fi; \
	[ "$$rc" = 0 ] || exit $$rc
	codesign --verify --strict "$(BAR_BUNDLE)"

print-dr:
	@codesign -d -r- "$(BUNDLE)" 2>&1 | grep -E '^designated|# designated' || true
	@codesign -dvvv "$(BUNDLE)" 2>&1 | grep -E "^CDHash=" || true

# cert: create the local code-signing identity (idempotent). Public cert only
# is kept on disk; key.pem and the p12 are deleted.
cert:
	@if security find-certificate -c tock-local-codesign >/dev/null 2>&1; then \
		echo "cert: tock-local-codesign already in keychain"; exit 0; \
	fi; \
	D=$$(mktemp -d) && chmod 700 "$$D" || exit 1; \
	rc=0; \
	{ \
		openssl req -x509 -newkey rsa:2048 -nodes -keyout "$$D/key.pem" -out "$$D/cert.pem" -days 3650 -config packaging/cs.cnf \
		&& P=$$(openssl rand -hex 16) \
		&& openssl pkcs12 -export -legacy -name tock-local-codesign -inkey "$$D/key.pem" -in "$$D/cert.pem" -out "$$D/tock.p12" -passout pass:$$P \
		&& security import "$$D/tock.p12" -k $(HOME)/Library/Keychains/login.keychain-db -P $$P -T /usr/bin/codesign -x \
		&& mkdir -p "$(dir $(CERT_PEM))" && chmod 700 "$(dir $(CERT_PEM))" \
		&& cp "$$D/cert.pem" "$(CERT_PEM)" && chmod 644 "$(CERT_PEM)"; \
	} || rc=$$?; \
	rm -rf "$$D"; \
	exit $$rc

# cert-trust: a person runs this (macOS authorization dialog). Never run by an executor.
cert-trust:
	security add-trusted-cert -r trustRoot -p codeSign -k $(HOME)/Library/Keychains/login.keychain-db "$(CERT_PEM)"
	@security find-identity -v -p codesigning | grep -q '"tock-local-codesign"' \
		&& echo "cert-trust: tock-local-codesign is a valid code-signing identity" \
		|| { echo "cert-trust: tock-local-codesign is still not a valid code-signing identity" >&2; exit 1; }

# dump: run the bundle itself as the responsible process (RESEARCH Pitfall 2).
dump:
	@out="$${DUMP_OUT:-}"; \
	if [ -z "$$out" ]; then \
		out=$$(tty 2>/dev/null) || { echo "not a terminal: set DUMP_OUT=/path/to/file" >&2; exit 1; }; \
	fi; \
	open -W -n -a "$(BUNDLE)" --stdout "$$out" --stderr "$$out" --args --dump

dump-bg:
	@[ -n "$(DUMP_OUT)" ] || { echo "dump-bg requires DUMP_OUT=/path/to/file" >&2; exit 1; }
	@install -m 600 /dev/null "$(DUMP_OUT)"
	open -n -g -a "$(BUNDLE)" --stdout "$(DUMP_OUT)" --stderr "$(DUMP_OUT)" --args --dump

dump-stop:
	@if pkill -INT -f '$(BUNDLE)/Contents/MacOS/[t]rackd --dump'; then \
		echo "dump-stop: sent SIGINT"; \
	else \
		echo "dump-stop: no dump process running (ok)"; \
	fi

# install: bootout, rebuild and sign the bundle, install the tock CLI, then
# bootstrap the agent (Pitfall 12: never replace the binary under a running
# agent). Both binaries come from one build so they agree on the schema.
# The menu bar app follows the same steps after trackd.
# Then write starter rules if there are none, and run tock doctor: it waits
# for trackd's first heartbeat and opens System Settings for missing grants
# (trackd itself shows the Accessibility prompt). A doctor problem does not
# fail the install.
install:
	-launchctl bootout gui/$$(id -u)/$(AGENT_LABEL) 2>/dev/null
	$(MAKE) bundle
	CGO_ENABLED=0 go build -o build/tock ./cmd/tock
	install -d -m 755 "$(BIN_DIR)"
	install -m 755 build/tock "$(BIN_DIR)/tock"
	"$(BUNDLE)/Contents/MacOS/trackd" install
	-launchctl bootout gui/$$(id -u)/$(BAR_ID) 2>/dev/null
	$(MAKE) bundle-bar
	"$(BAR_BUNDLE)/Contents/MacOS/tockbar" install
	@[ -f "$${TOCK_CONFIG_DIR:-$(HOME)/.config/tock}/rules.toml" ] || "$(BIN_DIR)/tock" init
	-"$(BIN_DIR)/tock" doctor --wait --open

# uninstall: unload the agent and remove its plist; tock.db and the log stay.
uninstall:
	@if [ -x "$(BUNDLE)/Contents/MacOS/trackd" ]; then \
		"$(BUNDLE)/Contents/MacOS/trackd" uninstall; \
	else \
		launchctl bootout gui/$$(id -u)/$(AGENT_LABEL) 2>/dev/null; \
		rm -f "$(HOME)/Library/LaunchAgents/$(AGENT_LABEL).plist"; \
		echo "uninstalled $(AGENT_LABEL) (data kept)"; \
	fi
	@if [ -x "$(BAR_BUNDLE)/Contents/MacOS/tockbar" ]; then \
		"$(BAR_BUNDLE)/Contents/MacOS/tockbar" uninstall; \
	else \
		launchctl bootout gui/$$(id -u)/$(BAR_ID) 2>/dev/null; \
		rm -f "$(HOME)/Library/LaunchAgents/$(BAR_ID).plist"; \
	fi

# install-linux: ryuk (Hyprland). Pure Go, no bundle or signing: trackd and
# tock go to ~/.local/bin and trackd runs as the systemd user unit
# trackd.service. Add the hypridle listener from the README for idle time.
install-linux:
	CGO_ENABLED=0 go build -ldflags "-X main.version=$(VERSION)" -o build/trackd ./cmd/trackd
	CGO_ENABLED=0 go build -o build/tock ./cmd/tock
	install -d -m 755 "$(BIN_DIR)"
	install -m 755 build/trackd "$(BIN_DIR)/trackd"
	install -m 755 build/tock "$(BIN_DIR)/tock"
	"$(BIN_DIR)/trackd" install
	@[ -f "$${TOCK_CONFIG_DIR:-$(HOME)/.config/tock}/rules.toml" ] || "$(BIN_DIR)/tock" init
	-"$(BIN_DIR)/tock" doctor --wait

# uninstall-linux: stop and remove the unit; data stays.
uninstall-linux:
	@if [ -x "$(BIN_DIR)/trackd" ]; then "$(BIN_DIR)/trackd" uninstall; \
	else systemctl --user disable --now trackd.service 2>/dev/null; rm -f "$${XDG_CONFIG_HOME:-$(HOME)/.config}/systemd/user/trackd.service"; fi
