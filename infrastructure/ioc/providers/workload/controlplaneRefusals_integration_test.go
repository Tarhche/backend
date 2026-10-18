//go:build integration

package workload

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	resourcesMongo "github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
)

// TestNewControlPlaneWorkload_refusedOnMongo holds the control plane, as it
// serves over resources kept in MongoDB, on the server MONGO_TEST_URI names,
// to refusing through its API what no record could keep as it is, as it
// refuses it over resources kept in memory: rather than failing to keep it.
func TestNewControlPlaneWorkload_refusedOnMongo(t *testing.T) {
	t.Parallel()

	uri := os.Getenv("MONGO_TEST_URI")
	if len(uri) == 0 {
		t.Skip("MONGO_TEST_URI names no database to test against")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	db := client.Database(fmt.Sprintf("controlplane_refused_%d", time.Now().UnixNano()))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	refused(t, resourcesMongo.NewRepository(db))
}
