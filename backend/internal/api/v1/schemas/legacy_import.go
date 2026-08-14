package schemas

import "github.com/murandido/m-80-backend/internal/legacyimport"

// The DTOs of the temporary spreadsheet importer. They are thin on purpose: the shapes the
// screen needs are the ones internal/legacyimport already produces, and restating a dozen
// nested structs here would be a second definition to keep in step for a feature that is
// meant to be deleted whole.

// LegacyImportDaySummary is one sheet of the workbook, as the day picker shows it.
type LegacyImportDaySummary struct {
	Date      string `json:"date" example:"2026-08-01"`
	SheetName string `json:"sheet_name" example:"01-08-26"`
	// RowCount is how many issuances the day holds, blank template rows and the
	// operators' own summary tables excluded.
	RowCount int `json:"row_count"`
	Blocks   int `json:"blocks"`
}

// LegacyImportPreviewResponse always lists the workbook's days; Preview is filled only
// when the request named one.
type LegacyImportPreviewResponse struct {
	Days    []LegacyImportDaySummary `json:"days"`
	Preview *legacyimport.DayPreview `json:"preview,omitempty"`
}

// The two envelope types below exist so the swagger annotations can name a concrete
// response shape, matching how the rest of the API documents itself.
type LegacyImportPreviewSingleResponse struct {
	Success bool                        `json:"success" example:"true"`
	Message string                      `json:"message" example:"Legacy import preview built"`
	Data    LegacyImportPreviewResponse `json:"data"`
}

type LegacyImportCommitSingleResponse struct {
	Success bool                      `json:"success" example:"true"`
	Message string                    `json:"message" example:"Legacy import committed"`
	Data    legacyimport.CommitResult `json:"data"`
}
