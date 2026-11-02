.PHONY: build test lint preflight clean fingerprint

build:
	GOWORK=off go build ./...

test:
	GOWORK=off go test -race -count=1 ./...

lint:
	GOWORK=off golangci-lint run ./...

preflight:
	@out=$$(GOWORK=off gofmt -l -s .); if [ -n "$$out" ]; then echo "$$out"; echo "gofmt failed"; exit 1; fi
	GOWORK=off go vet ./...
	GOWORK=off go build ./...
	GOWORK=off go test -race -count=1 ./...
	@out=$$(GOWORK=off go test -count=1 -v -run '^(TestLive_SetProxyRotation_DoesNotFailInFlightRequests|TestLive_CloseIdleConnections_ConcurrentWithDo)$$' . 2>&1) || { echo "$$out"; exit 1; }; \
	for t in TestLive_SetProxyRotation_DoesNotFailInFlightRequests TestLive_CloseIdleConnections_ConcurrentWithDo; do \
		echo "$$out" | grep -q -- "--- PASS: $$t " || { echo "$$out"; echo "non-race test $$t did not run"; exit 1; }; \
	done; echo "non-race tests ran: ok"

clean:
	go clean -cache -testcache

# fingerprint runs the TLS/HTTP2 fingerprint oracle (//go:build fingerprint).
# It is NOT part of preflight — it hits the network (peet.ws + browserleaks) and
# needs reference files in testdata/ captured by
# `go run ./cmd/fingerprint-capture -major <major>`.
# A failure means a go-stealth Chrome profile emits a fingerprint that differs
# from what a real Chrome of the same major emits — a true result, not a test
# defect. Fix the profile in a separate reviewed change; do not weaken the
# comparison to make it green.
fingerprint:
	GOWORK=off go test -tags fingerprint -count=1 -run 'TestFingerprintOracle|TestReferenceProvenance' ./...
