.PHONY: swagger run build

swagger:
	swag init -g main.go -o docs

run: swagger
	go run main.go

build: swagger
	go build -o rpanel main.go
