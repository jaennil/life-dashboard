// Command xiaomi-test dumps Xiaomi Body Composition Scale history straight
// from the Xiaomi Home cloud, using the same client as the xiaomi_scale
// connector. Useful for checking credentials and inspecting raw payloads
// without touching the database.
//
//	XIAOMI_ACCOUNT_ID=<numeric uid> XIAOMI_PASS_TOKEN=<token> go run ./cmd/xiaomi-test
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"life-dashboard/internal/connectors"
)

func main() {
	accountID := os.Getenv("XIAOMI_ACCOUNT_ID")
	passToken := os.Getenv("XIAOMI_PASS_TOKEN")
	region := envOr("XIAOMI_REGION", "ru")
	model := envOr("XIAOMI_MODEL", "yunmai.scales.ms104")
	outPath := os.Getenv("XIAOMI_OUT")

	if accountID == "" || passToken == "" {
		log.Fatal("set XIAOMI_ACCOUNT_ID and XIAOMI_PASS_TOKEN")
	}

	ctx := context.Background()
	cloud := connectors.NewXiaomiCloud(region)
	if err := cloud.Login(ctx, accountID, passToken); err != nil {
		log.Fatal("login: ", err)
	}
	log.Printf("logged in as %s (region %s)", accountID, region)

	records, err := cloud.FetchSince(ctx, model, time.Time{})
	if err != nil {
		log.Fatal("fetch: ", err)
	}
	log.Printf("fetched %d records", len(records))

	if len(records) > 0 {
		newest := records[0]
		log.Printf("newest: %s  %s", newest.MeasuredAt().Format(time.RFC3339), newest.Data)
		log.Printf("oldest: %s", records[len(records)-1].MeasuredAt().Format(time.RFC3339))
	}

	if outPath != "" {
		blob, _ := json.MarshalIndent(records, "", " ")
		if err := os.WriteFile(outPath, blob, 0o600); err != nil {
			log.Fatal("write: ", err)
		}
		log.Printf("wrote %s", outPath)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
