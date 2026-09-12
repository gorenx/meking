package sqlite

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/memoria-space/meking/controlplane"
	controlapplication "github.com/memoria-space/meking/controlplane/application"
	transactionsqlite "github.com/memoria-space/meking/transaction/adapter/sqlite"
	_ "modernc.org/sqlite"
)

func TestPoliciesPersistsProjectActionPolicy(t *testing.T) {
	store, transactions := openStore(t)
	service, err := controlapplication.NewPolicies(transactions, store)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	policy, err := service.Publish(t.Context(), controlapplication.PublishPolicyInput{
		Action:           controlplane.ConvertDocument,
		Mode:             controlplane.Automatic,
		MinimumPending:   3,
		MaximumWait:      time.Minute,
		ExpectedRevision: 0,
		PublishedAt:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.Revision != 1 {
		t.Fatalf("created Revision = %d, want 1", policy.Revision)
	}
	policy, err = service.Publish(t.Context(), controlapplication.PublishPolicyInput{
		Action:           controlplane.ConvertDocument,
		Mode:             controlplane.Manual,
		ExpectedRevision: 1,
		PublishedAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.Revision != 2 || policy.Mode != controlplane.Manual {
		t.Fatalf("updated Policy = %#v", policy)
	}
	_, err = service.Publish(t.Context(), controlapplication.PublishPolicyInput{
		Action:           controlplane.ConvertDocument,
		Mode:             controlplane.Suspended,
		ExpectedRevision: 1,
		PublishedAt:      now.Add(2 * time.Minute),
	})
	if !errors.Is(err, controlplane.ErrPolicyConflict) {
		t.Fatalf("stale Publish() error = %v, want ErrPolicyConflict", err)
	}
	stored, err := service.Policy(t.Context(), controlplane.ConvertDocument)
	if err != nil {
		t.Fatal(err)
	}
	if stored != policy {
		t.Fatalf("stored Policy = %#v, want %#v", stored, policy)
	}
}

func openStore(t *testing.T) (*Store, *transactionsqlite.Tx) {
	t.Helper()
	database, err := sql.Open(
		"sqlite",
		"file:"+t.Name()+"?mode=memory&cache=shared",
	)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	store, err := NewStore(database)
	if err != nil {
		t.Fatal(err)
	}
	transactions, err := transactionsqlite.New(database)
	if err != nil {
		t.Fatal(err)
	}
	return store, transactions
}
