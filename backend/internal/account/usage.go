package account

import (
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Usage is what an account has consumed of its program's beneficiary cap, right now.
//
// Nothing here is stored. The number is a window over account_beneficiary_uses, which is
// why nobody has to reset anything on 1 January: the Smiles count simply stops seeing last
// year's rows, and the LATAM one slides forward a day at a time.
type Usage struct {
	// Used is the count under the program's counting mode: distinct people for Smiles and
	// Azul, one per issuance for LATAM.
	Used int `json:"used"`
	// Limit is the program's cap.
	Limit int `json:"limit"`
	// Remaining is Limit - Used, floored at zero. Zero means the next issuance is the one
	// that gets the account blocked.
	Remaining int    `json:"remaining"`
	Window    string `json:"window"`
	Mode      string `json:"mode"`
	// ResetsAt is when Used goes back to zero, and only exists for a calendar-year cap: a
	// rolling window never resets, it expires row by row.
	ResetsAt *time.Time `json:"resets_at"`
}

// UsageFor computes one account's consumption, or nil when there is no number to show.
func UsageFor(db *gorm.DB, account data.Account, program data.LoyaltyProgram) (*Usage, error) {
	byAccount, err := UsageForMany(db,
		[]data.Account{account},
		map[string]data.LoyaltyProgram{program.ID: program},
	)
	if err != nil {
		return nil, err
	}
	return byAccount[account.ID], nil
}

// UsageForMany computes the consumption of a whole listing in one query, keyed by account id.
//
// An account is absent from the result when it has no number to show, which is not an error
// condition - it is most accounts:
//
//   - the account is not "standard": an injunction suspends the cap entirely, and an
//     unidentified account gives us no basis to claim one applies;
//   - the program has no researched limit.
//
// An absent entry is what tells the panel to render no counter at all, which is different
// from rendering a zero.
func UsageForMany(
	db *gorm.DB,
	accounts []data.Account,
	programsByID map[string]data.LoyaltyProgram,
) (map[string]*Usage, error) {
	usable := make([]data.Account, 0, len(accounts))
	ids := make([]string, 0, len(accounts))
	for _, account := range accounts {
		program, ok := programsByID[account.LoyaltyProgramID]
		if !ok || !capped(account, program) {
			continue
		}
		usable = append(usable, account)
		ids = append(ids, account.ID)
	}
	if len(usable) == 0 {
		return map[string]*Usage{}, nil
	}

	now := time.Now()
	yearStart, resetsAt := windowStart(data.CPFLimitWindowCalendarYear, now)
	rollingStart, _ := windowStart(data.CPFLimitWindowRolling12Months, now)

	counts, err := countUses(db, ids, yearStart, rollingStart)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*Usage, len(usable))
	for _, account := range usable {
		program := programsByID[account.LoyaltyProgramID]
		used := counts[account.ID].pick(*program.CPFLimitWindow, *program.CPFLimitMode)

		remaining := *program.CPFLimit - used
		if remaining < 0 {
			// Over the cap. Not impossible: the ledger is reconstructed from issuances
			// made long before the panel could warn about any of this.
			remaining = 0
		}

		usage := Usage{
			Used:      used,
			Limit:     *program.CPFLimit,
			Remaining: remaining,
			Window:    *program.CPFLimitWindow,
			Mode:      *program.CPFLimitMode,
		}
		if *program.CPFLimitWindow == data.CPFLimitWindowCalendarYear {
			usage.ResetsAt = resetsAt
		}
		result[account.ID] = &usage
	}
	return result, nil
}

func capped(account data.Account, program data.LoyaltyProgram) bool {
	return account.Category == data.AccountCategoryStandard &&
		program.CPFLimit != nil &&
		program.CPFLimitWindow != nil &&
		program.CPFLimitMode != nil
}

// windowStart returns the instant the current window opened, and when it closes if it ever
// does. Local time on purpose: the programs reset on the operator's calendar, not on UTC's.
func windowStart(window string, now time.Time) (since time.Time, resetsAt *time.Time) {
	if window == data.CPFLimitWindowRolling12Months {
		return now.AddDate(0, -12, 0), nil
	}

	// Calendar year, the default reading for anything else: an unrecognised window must not
	// silently become "unlimited".
	start := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, now.Location())
	next := start.AddDate(1, 0, 0)
	return start, &next
}

// accountCounts holds all four readings of the same rows, so a listing mixing Smiles and
// LATAM accounts still costs one query.
type accountCounts struct {
	DistinctYear    int `gorm:"column:distinct_year"`
	IssuancesYear   int `gorm:"column:issuances_year"`
	DistinctRolling int `gorm:"column:distinct_rolling"`
	IssuancesRolls  int `gorm:"column:issuances_rolling"`
}

func (c accountCounts) pick(window, mode string) int {
	perIssuance := mode == data.CPFLimitModePerIssuance
	if window == data.CPFLimitWindowRolling12Months {
		if perIssuance {
			return c.IssuancesRolls
		}
		return c.DistinctRolling
	}
	if perIssuance {
		return c.IssuancesYear
	}
	return c.DistinctYear
}

func countUses(
	db *gorm.DB,
	accountIDs []string,
	yearStart, rollingStart time.Time,
) (map[string]accountCounts, error) {
	// Both windows and both modes in one pass. The four aggregates read the same rows and
	// differ only in their FILTER, so this costs one scan rather than four queries.
	//
	// The boundaries are bound from Go rather than written as date_trunc('year', now()):
	// the cutoff has to be the same instant the ResetsAt shown on screen refers to, and
	// the database's timezone is not necessarily the operator's.
	// Flat, not an embedded accountCounts: GORM's Scan maps by column name onto the
	// struct's own fields and leaves an anonymous embedded struct untouched - which reads
	// as "this account has used nothing" rather than as an error.
	var rows []struct {
		AccountID       string `gorm:"column:account_id"`
		DistinctYear    int    `gorm:"column:distinct_year"`
		IssuancesYear   int    `gorm:"column:issuances_year"`
		DistinctRolling int    `gorm:"column:distinct_rolling"`
		IssuancesRolls  int    `gorm:"column:issuances_rolling"`
	}

	// The holder is excluded here rather than at write time: whether someone is the holder
	// can change (the account is transferred, the holder is registered late), and a filter
	// re-reads that on every count while a skipped write would be wrong forever.
	err := db.Table("account_beneficiaries AS beneficiaries").
		Select(`beneficiaries.account_id AS account_id,
			COUNT(DISTINCT uses.beneficiary_id) FILTER (WHERE uses.used_at >= @year)  AS distinct_year,
			COUNT(*)                            FILTER (WHERE uses.used_at >= @year)  AS issuances_year,
			COUNT(DISTINCT uses.beneficiary_id) FILTER (WHERE uses.used_at >= @rolling) AS distinct_rolling,
			COUNT(*)                            FILTER (WHERE uses.used_at >= @rolling) AS issuances_rolling`,
			map[string]any{"year": yearStart, "rolling": rollingStart}).
		Joins("JOIN account_beneficiary_uses AS uses ON uses.beneficiary_id = beneficiaries.id").
		Where("beneficiaries.account_id IN ?", accountIDs).
		Where("beneficiaries.is_holder = FALSE").
		Group("beneficiaries.account_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("account.UsageForMany: counting beneficiary uses: %w", err)
	}

	counts := make(map[string]accountCounts, len(rows))
	for _, row := range rows {
		counts[row.AccountID] = accountCounts{
			DistinctYear:    row.DistinctYear,
			IssuancesYear:   row.IssuancesYear,
			DistinctRolling: row.DistinctRolling,
			IssuancesRolls:  row.IssuancesRolls,
		}
	}
	return counts, nil
}
