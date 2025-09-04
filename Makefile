BINS = sbsh sb sbsh-session

all: $(BINS)

sbsh:
	go build -o sbsh ./cmd/sbsh

sb:
	go build -o sb ./cmd/sb

sbsh-session:
	go build -o sbsh-session ./cmd/sbsh-session

clean:
	rm -rf sbsh sb sbsh-session
