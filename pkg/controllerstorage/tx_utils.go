package controllerstorage

import (
	"database/sql"
	"errors"
	"log"
)

func rollbackTx(tx *sql.Tx, context string) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		log.Printf("WARN: rollback failed during %s: %v", context, err)
	}
}
