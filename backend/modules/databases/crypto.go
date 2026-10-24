package databases

import "rpanel/backend/secretbox"

// The cipher lives in the shared secretbox package (also used for 2FA secrets).
type Cipher = secretbox.Cipher

var (
	NewCipher   = secretbox.NewCipher
	ErrNoSecret = secretbox.ErrNoSecret
)
