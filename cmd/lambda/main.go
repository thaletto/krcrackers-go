package main

import (
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"github.com/thaletto/krcrackers-go/config"
	"github.com/thaletto/krcrackers-go/database"
	"github.com/thaletto/krcrackers-go/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := database.New(database.Config{
		Mode:  cfg.Backend,
		D1:    cfg.D1,
		Local: cfg.Local,
	})
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	handler := server.NewHandler(db)
	adapter := httpadapter.NewV2(handler)

	lambda.Start(adapter.ProxyWithContext)
}
