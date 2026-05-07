package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"market-project/backend/internal/aiserver"
	"market-project/backend/internal/cache"
	"market-project/backend/internal/config"
	"market-project/backend/internal/database"
	"market-project/backend/internal/services"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var embeddedDB *embeddedpostgres.EmbeddedPostgres
	dbPool, err := database.NewPool(ctx, cfg)
	if err != nil && cfg.EmbeddedPostgres {
		log.Printf("postgres unavailable, starting embedded postgres: %v", err)
		embeddedDB = embeddedpostgres.NewDatabase(embeddedDatabaseConfig(cfg))
		if startErr := embeddedDB.Start(); startErr != nil {
			if strings.Contains(startErr.Error(), "already listening on port") || strings.Contains(startErr.Error(), "another server might be running") || strings.Contains(startErr.Error(), "lock file") {
				log.Printf("embedded postgres already running, retrying connection: %v", startErr)
				time.Sleep(2 * time.Second)
			} else {
				log.Fatalf("start embedded postgres: %v", startErr)
			}
		}
		dbPool, err = database.NewPool(ctx, cfg)
	}
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer dbPool.Close()
	if embeddedDB != nil {
		defer embeddedDB.Stop()
	}

	if err := database.EnsureSchema(ctx, dbPool); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	if err := database.SeedDemoUser(ctx, dbPool, cfg.DemoEmail, cfg.DemoPassword); err != nil {
		log.Fatalf("seed demo user: %v", err)
	}

	repo := database.NewRepository(dbPool)
	redisClient := cache.New(cfg)
	eastmoneyClient := services.NewEastmoneyClient(cfg.EastmoneyTimeout)
	newsClient := services.NewNewsClient(cfg.NewsTimeout)
	aiClient := services.NewAIClient(cfg)

	router := aiserver.NewRouter(cfg, repo, eastmoneyClient, newsClient, aiClient, redisClient)

	httpServer := &http.Server{
		Addr:              ":" + cfg.AIPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("ai service listening on http://localhost:%s", cfg.AIPort)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve http: %v", err)
	}
}

func embeddedDatabaseConfig(cfg config.Config) embeddedpostgres.Config {
	parsed, err := url.Parse(cfg.DatabaseURL)
	if err != nil {
		return embeddedpostgres.DefaultConfig().
			Username("postgres").
			Password("postgres").
			Database("market_copilot").
			Port(5432).
			RuntimePath(cfg.EmbeddedRunPath).
			DataPath(cfg.EmbeddedDataPath)
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port == 0 {
		port = 5432
	}

	password, _ := parsed.User.Password()
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if databaseName == "" {
		databaseName = "market_copilot"
	}

	return embeddedpostgres.DefaultConfig().
		Username(parsed.User.Username()).
		Password(password).
		Database(databaseName).
		Port(uint32(port)).
		RuntimePath(cfg.EmbeddedRunPath).
		DataPath(cfg.EmbeddedDataPath)
}
