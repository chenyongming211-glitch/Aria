package token

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

func tokenRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "token", "tag", "tenant_id", "max_uses", "used_count", "expires_at", "created_at",
		"created_by", "status", "last_used_at", "last_used_by",
	})
}

func TestValidateAllowsMaxUsesZeroAsUnlimited(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, token, tag, COALESCE(tenant_id::text, ''), max_uses, used_count, expires_at, created_at,
		       COALESCE(created_by::text, ''), status, last_used_at, COALESCE(last_used_by::text, '')
		FROM tokens
		WHERE token = $1`)).
		WithArgs("tk_unlimited").
		WillReturnRows(tokenRows().AddRow(uuid.New(), "tk_unlimited", "unlimited", uuid.New().String(), 0, 99, now.Add(time.Hour), now, "", StatusActive, nil, "node-1"))

	tkn, err := NewValidator(NewStore(db)).Validate("tk_unlimited")
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if tkn == nil || tkn.MaxUses != 0 {
		t.Fatalf("expected unlimited token, got %#v", tkn)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestIncrementUsageDoesNotExhaustUnlimitedToken(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE tokens
		SET used_count = used_count + 1,
		    last_used_at = NOW(),
		    last_used_by = $2,
		    status = CASE
		        WHEN max_uses > 0 AND used_count + 1 >= max_uses THEN 'exhausted'
		        ELSE status
		    END
		WHERE token = $1 AND status = 'active' AND (max_uses = 0 OR used_count < max_uses)`)).
		WithArgs("tk_unlimited", "node-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := NewStore(db).IncrementUsage("tk_unlimited", "node-1"); err != nil {
		t.Fatalf("IncrementUsage returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
