package handlers

import (
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/api/middleware"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// errUnknownAgency is a grant naming an agency that does not exist.
var errUnknownAgency = errors.New("unknown agency")

// callerIsDev reports whether the authenticated user is a dev - the only role that decides
// which agencies an account sees, and the only one that may hand out the dev role itself.
// An admin who could do either would be setting the limits that are meant to bind them.
func callerIsDev(r *http.Request) bool {
	caller, ok := r.Context().Value(middleware.UserContextKey).(data.User)
	return ok && caller.Role == data.RoleDev
}

// loadUserAgencyIDs returns each user's granted agencies. Users without grants are absent.
func loadUserAgencyIDs(db *gorm.DB, userIDs []string) (map[string][]string, error) {
	var grants []data.UserAgency
	if len(userIDs) == 0 {
		return map[string][]string{}, nil
	}
	if err := db.Where("user_id IN ?", userIDs).Order("created_at, agency_id").Find(&grants).Error; err != nil {
		return nil, err
	}
	byUser := make(map[string][]string, len(userIDs))
	for _, grant := range grants {
		byUser[grant.UserID] = append(byUser[grant.UserID], grant.AgencyID)
	}
	return byUser, nil
}

// replaceUserAgencies makes the user's grants exactly agencyIDs. Run it inside the same
// transaction as the user write, so a bad agency id leaves the user untouched.
func replaceUserAgencies(tx *gorm.DB, userID string, agencyIDs []string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&data.UserAgency{}).Error; err != nil {
		return err
	}
	unique := slices.Compact(slices.Sorted(slices.Values(agencyIDs)))
	if len(unique) == 0 {
		return nil
	}
	grants := make([]data.UserAgency, 0, len(unique))
	for _, agencyID := range unique {
		grants = append(grants, data.UserAgency{UserID: userID, AgencyID: agencyID})
	}
	err := tx.Create(&grants).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresForeignKeyViolation {
		return errUnknownAgency
	}
	return err
}

func toUserResponse(user data.User, agencyIDs []string) schemas.UserResponse {
	if agencyIDs == nil {
		agencyIDs = []string{}
	}
	return schemas.UserResponse{
		ID:        user.ID,
		Name:      user.Name,
		Phone:     user.Phone,
		Role:      user.Role,
		AgencyIDs: agencyIDs,
		CreatedAt: user.CreatedAt,
	}
}
