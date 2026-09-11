package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/devseed"
	"github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/config"
	platformlogger "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/logger"
	platformmysql "github.com/yuanfengleeeeee/flight-collaboration-platform/internal/platform/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	configPath := flag.String("config", "configs/config.v2.yaml", "Architecture v2 config path")
	target := flag.String("target", "all", "seed target: core, edge or all")
	seed := flag.Int64("seed", devseed.DefaultSeed, "deterministic random seed")
	personnelCount := flag.Int("personnel", devseed.DefaultPersonnelCount, "number of demo personnel")
	flightCount := flag.Int("flights", devseed.DefaultFlightCount, "number of demo flights and task instances")
	prefix := flag.String("prefix", devseed.DefaultPrefix, "natural-key prefix for demo rows")
	password := flag.String("password", devseed.DefaultPassword, "shared demo employee password")
	flag.Parse()

	if *target != "core" && *target != "edge" && *target != "all" {
		fail("target must be core, edge or all")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(fmt.Sprintf("load config: %v", err))
	}
	log, err := platformlogger.New(cfg.Log)
	if err != nil {
		fail(fmt.Sprintf("initialize logger: %v", err))
	}
	defer log.Sync()

	options := devseed.Options{Seed: *seed, PersonnelCount: *personnelCount, FlightCount: *flightCount, Prefix: *prefix, Password: *password}
	ctx := context.Background()
	var manifest devseed.Manifest
	if *target == "core" || *target == "all" {
		db, err := platformmysql.Open(cfg.Core.DB)
		if err != nil {
			fail(fmt.Sprintf("open Core database: %v", err))
		}
		db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		manifest, err = devseed.SeedCore(ctx, db, options)
		_ = platformmysql.Close(db)
		if err != nil {
			fail(fmt.Sprintf("seed Core database: %v", err))
		}
		fmt.Printf("Core seed complete: personnel=%d projections=%d seed=%d prefix=%s\n", len(manifest.Personnel), len(manifest.Projections), manifest.Seed, manifest.Prefix)
	}
	if *target == "edge" {
		coreDB, err := platformmysql.Open(cfg.Core.DB)
		if err != nil {
			fail(fmt.Sprintf("open Core database for Edge source: %v", err))
		}
		coreDB = coreDB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		manifest, err = devseed.LoadManifestFromCore(ctx, coreDB, options)
		_ = platformmysql.Close(coreDB)
		if err != nil {
			fail(fmt.Sprintf("load Core seed source: %v", err))
		}
	}
	if *target == "edge" || *target == "all" {
		db, err := platformmysql.Open(cfg.Edge.DB)
		if err != nil {
			fail(fmt.Sprintf("open Edge database: %v", err))
		}
		db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		err = devseed.SeedEdge(ctx, db, manifest)
		_ = platformmysql.Close(db)
		if err != nil {
			fail(fmt.Sprintf("seed Edge database: %v", err))
		}
		fmt.Printf("Edge seed complete: projections=%d\n", len(manifest.Projections))
	}
	if *target == "core" || *target == "all" {
		fmt.Printf("Demo employee login: use the first numeric seeded employee_no and password=%s\n", *password)
		providerPrefix := strings.ToLower(strings.TrimSpace(*prefix))
		fmt.Printf("Demo provider codes: mock:%s-wx-0001 / mock:%s-wecom-0001\n", providerPrefix, providerPrefix)
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
