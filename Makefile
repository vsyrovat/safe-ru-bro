.PHONY: build run clean

build:
	go build -o safe-ru-bro .

run: build
	./safe-ru-bro

clean:
	rm -f safe-ru-bro
