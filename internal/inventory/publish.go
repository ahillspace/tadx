package inventory

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/cache"
)

// PublishDetail attempts one confirmed detail write-through without changing the
// result of the live read when the optional cache is unavailable.
func PublishDetail(store *cache.Store, entry cache.ResourceEntry) {
	if store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = store.UpsertResources(ctx, []cache.ResourceEntry{entry})
}
