package export

import (
	"bytes"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/xuri/excelize/v2"
)

func renderLegacyForTest(t *testing.T, issuances []data.Issuance) *excelize.File {
	t.Helper()

	out, err := renderLegacyIssuancesXLSX(legacyIssuanceTemplate, issuances, time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("renderLegacyIssuancesXLSX: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen rendered file: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// The legacy sheet fills the embedded MODELO: its B:P columns, plus the TAXA RESGATE column
// the template does not have.
func TestRenderLegacyIssuancesXLSXLayout(t *testing.T) {
	exempt := sampleIssuance("Smiles", "ABC123", "Conta VIP", "João Emissor")
	exempt.RedemptionFeeSavedCents = intptr(4200)
	exempt.Agency = &data.Agency{Name: "Agência Central"}

	f := renderLegacyForTest(t, []data.Issuance{
		exempt,
		sampleIssuance("Smiles", "DEF456", "Conta Comum", "João Emissor"),
	})

	assertCells(t, f, legacyIssuanceSheet, map[string]string{
		"B2": "COMPANHIA",
		"B3": "Smiles",
		"C3": "ABC123",
		"F3": "Ana Souza",
		"G3": "GRU",
		"H3": "SDU",
		"I3": "10.000",
		"M3": "Conta VIP",
		"O3": "João Emissor",
		"P3": "Agência Central",
		"Q2": "TAXA RESGATE",
		"Q3": "R$ 42.00",
		// An ordinary account saved nothing, so the cell stays blank.
		"Q4": "",
	})
}

// A booking cancelled on a later day enters by its refund only, as on the control sheet.
func TestRenderLegacyIssuancesXLSXRefundOnlyOnLaterCancellation(t *testing.T) {
	cancelledAt := time.Date(2026, 7, 25, 18, 0, 0, 0, time.UTC)

	issuance := sampleIssuance("Smiles", "ABC123", "Conta VIP", "João Emissor")
	issuance.CancelledAt = &cancelledAt
	issuance.RefundedPoints = intptr(10000)
	issuance.RefundedTaxes = intptr(3550)
	issuance.RedemptionFeeSavedCents = intptr(4200)

	f := renderLegacyForTest(t, []data.Issuance{issuance})

	assertCells(t, f, legacyIssuanceSheet, map[string]string{
		"I3": "-10.000",
		"K3": "-R$ 35.50",
		"Q3": "-R$ 42.00",
	})
}
