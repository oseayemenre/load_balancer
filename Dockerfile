FROM golang:1.25

WORKDIR /app

COPY go.mod go.sum .

RUN go mod download

COPY . .

ENV PORT=3000

ENTRYPOINT ["go", "run", "cmd/main.go"]