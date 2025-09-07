BINS = sbsh sb sbsh-session

all: clean kill $(BINS)

sbsh:
	go build -o sbsh ./cmd/sbsh

sb:
	go build -o sb ./cmd/sb

sbsh-session:
	go build -o sbsh-session ./cmd/sbsh-session

clean:
	rm -rf sbsh sb sbsh-session

kill:
	(killall sbsh || true ); (killall sbsh-session || true)