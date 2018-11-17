all: bin/mtool test

build: bin/mtool

bin/mtool: *.go
	mkdir -p bin/
	GOBIN=$(CURDIR)/bin go install .

test: bin/mtool
	git ls-files --stage > testcase1.txt
	$(CURDIR)/bin/mtool < testcase1.txt > $(CURDIR)/mtool1.txt
	$(CURDIR)/bin/mtool --verify --verbose --snapshot=$(CURDIR)/mtool1.txt < testcase1.txt

clean:
	rm -f bin/mtool

.PHONY: all test build clean
