BINS = sbsh sb

all: clean kill $(BINS)

sbsh:
	go build -o sbsh ./cmd/sbsh

sb:
	go build -o sb ./cmd/sb

clean:
	rm -rf sbsh sb sbsh-session

kill:
	(killall sbsh || true )

test:
	go test ./cmd/sbsh...
	go test ./pkg/session...
	go test ./pkg/supervisor...
