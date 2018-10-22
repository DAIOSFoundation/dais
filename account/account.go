package account

import (
	"../types"
)

type Account struct {
	Address types.Address `json:"address"`
}

type Wallet interface {
	Status() (string, error)

	Open(passphrase string) error

	Close() error

	Accounts() []Account
}
