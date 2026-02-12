package utils

import "regexp"

func ClearPhone(phone string) string {
	// removes everything that isn't a number
	re := regexp.MustCompile(`\D`)

	return re.ReplaceAllString(phone, "")
}

func ValidateBRPhone(phone string) bool {
	cleanPhone := ClearPhone(phone)

	// should have 10-11 numbers (DDD + number)
	if len(cleanPhone) < 10 || len(cleanPhone) > 11 {
		return false
	}

	regex := regexp.MustCompile(`^([1-9]{2})(9?[0-9]{8})$`)
	return regex.MatchString(cleanPhone)
}
