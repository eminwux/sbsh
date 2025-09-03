BINS = sbsh sb

all: $(BINS)

sbsh:
	go build -o sbsh ./cmd/sbsh

sb:
	go build -o sb ./cmd/sb

clean:
	rm -rf sbsh sb
