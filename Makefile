pc:
	go run cmd/pc/main.go

tr: clean
	go run cmd/tr/main.go

clean:
	@rm -rf data/output/*
