package account

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// A holder and their loyalty accounts answer to one list of aliases. The panel shows them as
// one row, and the operators name them the same way in a WhatsApp message whatever the
// program, so an alias written on one side has to reach the other: "Heraldo" registered on the
// person is also what the bot must find on Heraldo's Smiles and Azul accounts, and an alias
// the bot learns on one account is also how the person is known from then on.
//
// The two tables stay - person_nicknames is what line 4 of an EMISSAO resolves the
// administrator against, account_nicknames is what line 2 resolves the account against, and
// each has its own uniqueness (global for people, per program for accounts). What this file
// keeps is the invariant between them: every alias of a person is an alias of each live
// account they hold, and the other way round.
//
// Matching is case-insensitive, the same way the bot matches, so "heraldo" and "Heraldo" are
// one alias and the spelling first written is the one kept.

// NicknameTakenError is an alias that cannot be shared because somebody else already
// answers to it - another person, or another account in the same program.
type NicknameTakenError struct {
	Nickname string
	// Where names the conflict for the operator: "outra pessoa" or the program's name.
	Where string
}

func (e *NicknameTakenError) Error() string {
	if e.Where == "" {
		return fmt.Sprintf("O apelido %q já é de outra pessoa", e.Nickname)
	}
	return fmt.Sprintf("O apelido %q já é de outra conta na %s", e.Nickname, e.Where)
}

// heldAccounts are the live accounts a person holds, with their programs for the messages.
func heldAccounts(tx *gorm.DB, personID string) ([]data.Account, error) {
	var accounts []data.Account
	err := tx.Preload("LoyaltyProgram").Where("holder_id = ?", personID).Find(&accounts).Error
	return accounts, err
}

func personHasNickname(tx *gorm.DB, personID, nickname string) (bool, error) {
	var count int64
	err := tx.Model(&data.PersonNickname{}).
		Where("person_id = ? AND LOWER(nickname) = LOWER(?)", personID, nickname).
		Count(&count).Error
	return count > 0, err
}

// personNicknameTakenByOther reports whether some other person already answers to the alias.
func personNicknameTakenByOther(tx *gorm.DB, personID, nickname string) (bool, error) {
	var count int64
	err := tx.Model(&data.PersonNickname{}).
		Where("person_id <> ? AND LOWER(nickname) = LOWER(?)", personID, nickname).
		Count(&count).Error
	return count > 0, err
}

func accountHasNickname(tx *gorm.DB, accountID, nickname string) (bool, error) {
	var count int64
	err := tx.Model(&data.AccountNickname{}).
		Where("account_id = ? AND LOWER(nickname) = LOWER(?)", accountID, nickname).
		Count(&count).Error
	return count > 0, err
}

func accountNicknameTakenByOther(tx *gorm.DB, account data.Account, nickname string) (bool, error) {
	var count int64
	err := tx.Model(&data.AccountNickname{}).
		Where("loyalty_program_id = ? AND account_id <> ? AND LOWER(nickname) = LOWER(?)",
			account.LoyaltyProgramID, account.ID, nickname).
		Count(&count).Error
	return count > 0, err
}

func programName(account data.Account) string {
	if account.LoyaltyProgram != nil {
		return account.LoyaltyProgram.Name
	}
	return "mesmo programa"
}

// ShareNickname registers an alias on a person and on every live account they hold, skipping
// whichever already has it. It is all or nothing: if any side is taken by somebody else the
// whole write is refused with a NicknameTakenError, because an alias that reaches some of a
// holder's accounts and not others is exactly the drift this exists to prevent.
//
// Run it inside the caller's transaction.
func ShareNickname(tx *gorm.DB, personID, nickname string) error {
	nickname = strings.TrimSpace(nickname)

	has, err := personHasNickname(tx, personID, nickname)
	if err != nil {
		return err
	}
	if !has {
		taken, err := personNicknameTakenByOther(tx, personID, nickname)
		if err != nil {
			return err
		}
		if taken {
			return &NicknameTakenError{Nickname: nickname}
		}
		if err := tx.Create(&data.PersonNickname{PersonID: personID, Nickname: nickname}).Error; err != nil {
			return err
		}
	}

	accounts, err := heldAccounts(tx, personID)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		has, err := accountHasNickname(tx, account.ID, nickname)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		taken, err := accountNicknameTakenByOther(tx, account, nickname)
		if err != nil {
			return err
		}
		if taken {
			return &NicknameTakenError{Nickname: nickname, Where: programName(account)}
		}
		if err := tx.Create(&data.AccountNickname{
			AccountID:        account.ID,
			LoyaltyProgramID: account.LoyaltyProgramID,
			Nickname:         nickname,
		}).Error; err != nil {
			return err
		}
	}

	return nil
}

// UnshareNickname removes an alias from a person and from every live account they hold.
//
// The person's row is deleted; the accounts' are archived, because an issuance points at the
// account alias it came in by (issuances.account_nickname_id) and a delete would blank that.
func UnshareNickname(tx *gorm.DB, personID, nickname string) error {
	if err := tx.Where("person_id = ? AND LOWER(nickname) = LOWER(?)", personID, nickname).
		Delete(&data.PersonNickname{}).Error; err != nil {
		return err
	}

	return tx.Model(&data.AccountNickname{}).
		Where("LOWER(nickname) = LOWER(?)", nickname).
		Where("account_id IN (?)", tx.Model(&data.Account{}).Select("id").Where("holder_id = ?", personID)).
		UpdateColumn("deleted_at", time.Now()).Error
}

// UnifyHolderNicknames makes a person's aliases and their accounts' aliases the same list
// again, after something brought new ones together: an account created or handed to them, a
// merge, an alias the bot learned on one side. Unlike ShareNickname it never refuses - an
// alias somebody else already holds is left where it is and logged, since the write that
// triggered this has already happened and is not the operator's to undo.
func UnifyHolderNicknames(tx *gorm.DB, personID string) error {
	accounts, err := heldAccounts(tx, personID)
	if err != nil {
		return err
	}

	var personNicknames []data.PersonNickname
	if err := tx.Where("person_id = ?", personID).Order("created_at").Find(&personNicknames).Error; err != nil {
		return err
	}

	// Every spelling in the union, first one written wins.
	seen := map[string]bool{}
	var union []string
	add := func(nickname string) {
		key := strings.ToLower(strings.TrimSpace(nickname))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		union = append(union, strings.TrimSpace(nickname))
	}
	for _, nickname := range personNicknames {
		add(nickname.Nickname)
	}
	for _, account := range accounts {
		var accountNicknames []data.AccountNickname
		if err := tx.Where("account_id = ?", account.ID).Order("created_at").Find(&accountNicknames).Error; err != nil {
			return err
		}
		for _, nickname := range accountNicknames {
			add(nickname.Nickname)
		}
	}

	for _, nickname := range union {
		has, err := personHasNickname(tx, personID, nickname)
		if err != nil {
			return err
		}
		if !has {
			taken, err := personNicknameTakenByOther(tx, personID, nickname)
			if err != nil {
				return err
			}
			if taken {
				slog.Warn("alias not shared with the holder: another person answers to it",
					"person_id", personID, "nickname", nickname)
			} else if err := tx.Create(&data.PersonNickname{PersonID: personID, Nickname: nickname}).Error; err != nil {
				return err
			}
		}

		for _, account := range accounts {
			has, err := accountHasNickname(tx, account.ID, nickname)
			if err != nil {
				return err
			}
			if has {
				continue
			}
			taken, err := accountNicknameTakenByOther(tx, account, nickname)
			if err != nil {
				return err
			}
			if taken {
				slog.Warn("alias not shared with the account: another account in the program answers to it",
					"account_id", account.ID, "nickname", nickname)
				continue
			}
			if err := tx.Create(&data.AccountNickname{
				AccountID:        account.ID,
				LoyaltyProgramID: account.LoyaltyProgramID,
				Nickname:         nickname,
			}).Error; err != nil {
				return err
			}
		}
	}

	return nil
}
