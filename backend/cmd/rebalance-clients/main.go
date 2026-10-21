// Command rebalance-clients replays every client's payments once, so the rule that a sale's
// surplus is the client's credit (migration 85) reaches the sales that were already stuck
// on "Devolver" before it shipped. The replay runs by itself on every later change to a
// client's sales; this is only for the ones nobody touches again.
//
// It is a dry run unless -apply is given: each client is replayed inside a transaction that
// is rolled back, and what would change is printed. The replay writes only differences, so
// running it twice, or after the panel already replayed a client, changes nothing more.
//
// In production, after the deploy that carries migration 85:
//
//	docker compose --profile prod run --rm --no-deps api-prod ./tools/rebalance-clients
//	docker compose --profile prod run --rm --no-deps api-prod ./tools/rebalance-clients -apply
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
	"github.com/murandido/m-80-backend/internal/supplierreport"
	"gorm.io/gorm"
)

// errDryRun rolls a client's transaction back once its changes were printed.
var errDryRun = errors.New("dry run")

type saleState struct {
	ID             string
	ProgramLocator string
	PaidCents      int
}

func main() {
	apply := flag.Bool("apply", false, "write the changes instead of only printing them")
	flag.Parse()

	data.Connect()

	var clients []data.Client
	if err := data.DB.Select("id", "name").Order("name").Find(&clients).Error; err != nil {
		log.Fatal("❌ Failed to list the clients: ", err)
	}

	changedClients := 0
	for _, client := range clients {
		err := data.DB.Transaction(func(tx *gorm.DB) error {
			changed, err := rebalance(tx, client)
			if err != nil {
				return err
			}
			if changed {
				changedClients++
			}
			if !*apply {
				return errDryRun
			}
			return nil
		})
		if err != nil && !errors.Is(err, errDryRun) {
			log.Fatalf("❌ Failed to rebalance %s (%s): %v", client.Name, client.ID, err)
		}
	}

	mode := "would change (dry run, nothing written; pass -apply to write)"
	if *apply {
		mode = "changed"
	}
	fmt.Printf("\n%d of %d clients %s\n", changedClients, len(clients), mode)
}

// rebalance replays one client and prints what moved: each sale whose paid amount changed,
// and the credit before and after.
func rebalance(tx *gorm.DB, client data.Client) (bool, error) {
	before, err := salesOf(tx, client.ID)
	if err != nil {
		return false, err
	}
	creditBefore, err := sale.CreditOf(tx, client.ID)
	if err != nil {
		return false, err
	}

	saleIDs, err := sale.RebalanceClient(tx, client.ID)
	if err != nil || len(saleIDs) == 0 {
		return false, err
	}

	after, err := salesOf(tx, client.ID)
	if err != nil {
		return false, err
	}
	creditAfter, err := sale.CreditOf(tx, client.ID)
	if err != nil {
		return false, err
	}

	fmt.Printf("%s (%s)\n", client.Name, client.ID)
	for _, saleID := range saleIDs {
		was, now := before[saleID], after[saleID]
		fmt.Printf("  %-8s pago %s -> %s\n", now.ProgramLocator,
			supplierreport.FormatBRL(was.PaidCents), supplierreport.FormatBRL(now.PaidCents))
	}
	fmt.Printf("  crédito %s -> %s\n", supplierreport.FormatBRL(creditBefore), supplierreport.FormatBRL(creditAfter))
	return true, nil
}

func salesOf(tx *gorm.DB, clientID string) (map[string]saleState, error) {
	var rows []saleState
	err := tx.Table("sales").
		Select(`sales.id, issuances.program_locator,
			COALESCE((SELECT SUM(amount_cents) FROM sale_payments WHERE sale_id = sales.id), 0) AS paid_cents`).
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Where("sales.client_id = ?", clientID).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	states := make(map[string]saleState, len(rows))
	for _, row := range rows {
		states[row.ID] = row
	}
	return states, nil
}
