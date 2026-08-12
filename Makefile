GO ?= go
COUNT ?= 10

.PHONY: bench stat test shapes tidy clean

bench:
	$(GO) test -run '^$$' -bench . -benchmem -count=$(COUNT) -timeout 30m . | tee bench.txt

stat: bench.txt
	benchstat bench.txt

test:
	$(GO) test ./...

shapes:
	$(GO) test -run TestDynamicShapes -v .

tidy:
	$(GO) mod tidy

clean:
	rm -f bench.txt
