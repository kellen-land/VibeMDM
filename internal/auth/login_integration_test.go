package auth

import (
	"context"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vibemdm/vibemdm/internal/db"
	"github.com/vibemdm/vibemdm/internal/store"
)

func TestParallelLoginsCannotOutrunLockout(t *testing.T) {
	dsn := os.Getenv("VIBEMDM_TEST_DSN")
	if dsn == "" {
		t.Skip("set VIBEMDM_TEST_DSN to a throwaway PostgreSQL database to run integration tests")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := db.Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	st := store.New(d)
	name := "lockout-" + time.Now().Format("150405.000000")
	hash, _ := HashPassword("correct horse battery")
	if _, err := st.CreateUser(ctx, name, name, "", "viewer", hash); err != nil {
		t.Fatal(err)
	}
	a := New(st, false, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
			a.Login(httptest.NewRecorder(), r, name, "wrong password")
		}()
	}
	wg.Wait()
	a.limiter.mu.Lock()
	n := a.limiter.fails["u:"+name].count
	a.limiter.mu.Unlock()
	if n > loginBurst {
		t.Fatalf("%d password checks ran for one username; lockout should stop them at %d", n, loginBurst)
	}
}
