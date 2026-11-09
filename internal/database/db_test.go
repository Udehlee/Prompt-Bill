package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestConnect_UnreachableHost(t *testing.T) {
	// Use a 100ms deadline to verify ping context timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Connecting to a blackhole IP should fail via context deadline
	dsn := "postgres://tracker:tracker_pass@192.0.2.1:5432/tracker_dev?sslmode=disable&connect_timeout=1"
	db, err := Connect(ctx, dsn)
	if err == nil {
		if db != nil {
			_ = db.Close()
		}
		t.Fatal("expected connection to unreachable host to fail, got nil")
	}

	if !strings.Contains(err.Error(), "database ping failed") {
		t.Fatalf("expected ping failure error message, got: %v", err)
	}
}
