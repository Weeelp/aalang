TARGET = program.json

all:
	go run cmd/main/main.go

trans: clean
	go run cmd/translator/main.go

clean:
	@rm -f $(TARGET)
