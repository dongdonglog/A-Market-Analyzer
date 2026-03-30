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

	"market-project/backend/internal/cache"
	"market-project/backend/internal/config"
	"market-project/backend/internal/database"
	"market-project/backend/internal/server"
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
	if err := database.SeedRedeemCode(ctx, dbPool, "WELCOME100", 10000, 1000); err != nil {
		log.Fatalf("seed redeem code: %v", err)
	}
	if err := database.SeedMembershipRedeemCode(ctx, dbPool, "STARTER30", "starter", "Starter", 10000, 30, 1000); err != nil {
		log.Fatalf("seed membership redeem code: %v", err)
	}
	if err := database.SeedMembershipRedeemCode(ctx, dbPool, "ACTIVE30", "active", "Active", 40000, 30, 1000); err != nil {
		log.Fatalf("seed membership redeem code: %v", err)
	}
	if err := database.SeedMembershipRedeemCode(ctx, dbPool, "PRO30", "pro", "Pro", 140000, 30, 1000); err != nil {
		log.Fatalf("seed membership redeem code: %v", err)
	}

	repo := database.NewRepository(dbPool)
	redisClient := cache.New(cfg)
	eastmoneyClient := services.NewEastmoneyClient(cfg.EastmoneyTimeout)
	go syncSymbolCatalogLoop(ctx, repo, eastmoneyClient)

	router := server.NewRouter(cfg, repo, eastmoneyClient, redisClient)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("backend listening on http://localhost:%s", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve http: %v", err)
	}
}

func syncSymbolCatalogLoop(ctx context.Context, repo *database.Repository, eastmoneyClient *services.EastmoneyClient) {
	runSync := func() {
		syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		symbols, err := eastmoneyClient.FetchSymbolCatalog(syncCtx)
		if err != nil {
			log.Printf("sync symbol catalog: fetch failed: %v", err)
			return
		}

		if err := repo.UpsertSymbolCatalog(syncCtx, symbols); err != nil {
			log.Printf("sync symbol catalog: upsert failed: %v", err)
			return
		}

		log.Printf("sync symbol catalog: upserted %d symbols", len(symbols))
	}

	runSync()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runSync()
		}
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
