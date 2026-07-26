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

// NormalizeBRPhone inserts Brazil's optional mobile "nono dígito" when it's
// missing and safe to infer: an 8-digit local number (after the DDD)
// starting with 6-9 is a mobile number that dropped its leading 9
// (historical mobile prefixes), while 2-5 is a landline, which never had a
// 9th digit and must stay untouched. Already-11-digit numbers and anything
// else pass through as-is - this only resolves the one real ambiguity.
func NormalizeBRPhone(phone string) string {
	digits := ClearPhone(phone)
	if len(digits) == 10 && digits[2] >= '6' && digits[2] <= '9' {
		return digits[:2] + "9" + digits[2:]
	}
	return digits
}
