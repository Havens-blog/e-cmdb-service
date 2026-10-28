package dao

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestListByIDsFilter_TenantFiltering(t *testing.T) {
	ids := []int64{1, 2, 3}

	filtered := listByIDsFilter(ids, 100)
	want := bson.M{
		"id":        bson.M{"$in": ids},
		"tenant_id": int64(100),
	}
	if !reflect.DeepEqual(filtered, want) {
		t.Fatalf("listByIDsFilter(ids, 100) = %#v, want %#v", filtered, want)
	}

	unfiltered := listByIDsFilter(ids, 0)
	wantUnfiltered := bson.M{"id": bson.M{"$in": ids}}
	if !reflect.DeepEqual(unfiltered, wantUnfiltered) {
		t.Fatalf("listByIDsFilter(ids, 0) = %#v, want %#v", unfiltered, wantUnfiltered)
	}
}

func TestAggregateStatsByIDsMatch_TenantFiltering(t *testing.T) {
	ids := []int64{1, 2, 3}

	filtered := aggregateStatsByIDsMatch(ids, 200)
	want := bson.M{
		"id":        bson.M{"$in": ids},
		"tenant_id": int64(200),
	}
	if !reflect.DeepEqual(filtered, want) {
		t.Fatalf("aggregateStatsByIDsMatch(ids, 200) = %#v, want %#v", filtered, want)
	}

	unfiltered := aggregateStatsByIDsMatch(ids, 0)
	wantUnfiltered := bson.M{"id": bson.M{"$in": ids}}
	if !reflect.DeepEqual(unfiltered, wantUnfiltered) {
		t.Fatalf("aggregateStatsByIDsMatch(ids, 0) = %#v, want %#v", unfiltered, wantUnfiltered)
	}
}
