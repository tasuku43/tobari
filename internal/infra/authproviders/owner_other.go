//go:build !unix

package authproviders

import (
	"fmt"
	"os"
)

const currentUserOwnershipSupported = false

func validateCurrentUserOwner(_ os.FileInfo) error {
	return fmt.Errorf("user provider ownership checks are unsupported on this platform")
}
