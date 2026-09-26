package update

import (
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

func TestDatabaseTagDeltaObservationAndStableDeduplication(t *testing.T) {
	for _, observed := range []bool{false, true} {
		in := DatabaseInput{AddTags: []string{"existing", "new", "new", "later"}, RemoveTags: []string{"absent", "present", "present"}}
		v := value.MetadataDatabase{Tags: []string{"existing", "present"}, TagsObserved: observed}
		_, changes, add, remove := databaseChanged(v, in)
		wantAdd, wantRemove := []string{"existing", "new", "later"}, []string{"absent", "present"}
		if observed {
			wantAdd, wantRemove = []string{"new", "later"}, []string{"present"}
		}
		wantChanges := []Change{}
		for _, tag := range wantAdd {
			wantChanges = append(wantChanges, Change{Property: "add_tag", After: tag})
		}
		for _, tag := range wantRemove {
			wantChanges = append(wantChanges, Change{Property: "remove_tag", After: tag})
		}
		if !reflect.DeepEqual(add, wantAdd) || !reflect.DeepEqual(remove, wantRemove) || !reflect.DeepEqual(changes, wantChanges) {
			t.Fatalf("observed=%v: add=%v remove=%v changes=%+v", observed, add, remove, changes)
		}
	}
	_, changes, add, remove := databaseChanged(value.MetadataDatabase{}, DatabaseInput{})
	if changes == nil || add == nil || remove == nil {
		t.Fatal("empty changes and tag deltas must remain non-nil")
	}
}

func TestDatabaseRetainedIdentityPreservesOnlyMissingFields(t *testing.T) {
	previous := value.MetadataIdentity{MetadataID: "metadata", LUID: "rest", Name: "Previous", Type: "database"}
	if got := retainedIdentity(previous, value.MetadataIdentity{}); got != previous {
		t.Fatalf("missing fields lost: %+v", got)
	}
	observed := value.MetadataIdentity{MetadataID: "fresh-metadata", LUID: "fresh-rest", Name: "Fresh", Type: "table"}
	if got := retainedIdentity(previous, observed); got != observed {
		t.Fatalf("observed fields overwritten: %+v", got)
	}
}
