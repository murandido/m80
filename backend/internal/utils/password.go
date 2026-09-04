package utils

import (
	"runtime"

	"golang.org/x/crypto/bcrypt"
)

// bcrypt is deliberately expensive, and the API runs on two OCPUs. That combination
// has a failure mode that only shows up under load: a burst of logins does not merely
// make the logins slow, it makes everything slow.
//
// Measured on a replica of production (2 OCPU, 12 GB, aarch64), 100 operators logging
// in at the same moment pinned the API container at 196% CPU for eight seconds - both
// cores hashing, nothing left over. Requests that were only reading a paginated list
// went from 24 ms time-to-first-byte to 3.3 s, and 24 logins sat in the queue past the
// 60 s client timeout. The database was idle at 0.5% the whole time: the contention is
// purely CPU, and purely bcrypt.
//
// hashGate bounds how many hashes run concurrently so at least one core is always free
// to serve the requests that are not logins. Logins still queue - the last person in a
// burst of 100 waits behind the other 99 - but they queue against one core instead of
// stalling every other request on the box. Slow login for the people logging in beats
// slow everything for the people already working.
var hashGate = make(chan struct{}, maxConcurrentHashes())

// maxConcurrentHashes leaves one core for everything that is not hashing. On a
// single-core machine there is nothing to reserve, so the gate degrades to serialising
// the hashes, which is still better than letting them fight the request loop.
func maxConcurrentHashes() int {
	if reserved := runtime.GOMAXPROCS(0) - 1; reserved > 0 {
		return reserved
	}
	return 1
}

// HashPassword returns the bcrypt hash of password, waiting its turn at the gate.
//
// The wait is deliberately unbounded. A timeout here would turn "your login took eight
// seconds" into "your login failed", and the queue is short-lived by construction: it
// drains at roughly ten hashes per second per reserved core.
func HashPassword(password string) (string, error) {
	hashGate <- struct{}{}
	defer func() { <-hashGate }()

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// CheckPassword reports whether password matches hash, waiting its turn at the gate.
//
// Comparing costs the same as hashing - bcrypt derives the candidate hash with the cost
// and salt read out of the stored one - so the verify path is the one a login storm
// actually hammers, and it has to pass through the same gate.
func CheckPassword(hash, password string) bool {
	hashGate <- struct{}{}
	defer func() { <-hashGate }()

	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
