// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pagination

import (
	"net/url"
	"reflect"
	"testing"
)

type item struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Score int    `json:"score"`
}

func mkItems(n int) []item {
	out := make([]item, n)
	for i := 0; i < n; i++ {
		out[i] = item{
			ID:    string(rune('z' - i%26)),
			Name:  string(rune('a' + i%26)),
			Score: n - i,
		}
	}
	return out
}

func TestRequestFromQuery_Defaults(t *testing.T) {
	r := RequestFromQuery(url.Values{})
	if r.Limit != DefaultLimit || r.Offset != 0 {
		t.Fatalf("unexpected defaults: %+v", r)
	}
}

func TestRequestFromQuery_ClampsLimit(t *testing.T) {
	q := url.Values{"limit": {"50000"}}
	r := RequestFromQuery(q)
	if r.Limit != MaxLimit {
		t.Fatalf("expected limit clamped to %d, got %d", MaxLimit, r.Limit)
	}
}

func TestRequestFromQuery_RejectsNegativeAndZero(t *testing.T) {
	r := RequestFromQuery(url.Values{"limit": {"0"}, "offset": {"-5"}})
	if r.Limit != DefaultLimit {
		t.Fatalf("limit=0 should fall back to default, got %d", r.Limit)
	}
	if r.Offset != 0 {
		t.Fatalf("negative offset should fall back to 0, got %d", r.Offset)
	}
}

func TestPaginate_SliceFirstPage(t *testing.T) {
	data := mkItems(250)
	page, err := Paginate(data, Request{Limit: 100, Offset: 0}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !page.Paginated {
		t.Fatal("expected paginated=true")
	}
	if page.Meta.Total != 250 {
		t.Fatalf("total=%d", page.Meta.Total)
	}
	if page.Meta.Returned != 100 {
		t.Fatalf("returned=%d", page.Meta.Returned)
	}
	if !page.Meta.HasMore {
		t.Fatal("has_more should be true")
	}
	got := page.Items.([]item)
	if len(got) != 100 {
		t.Fatalf("len(items)=%d", len(got))
	}
}

func TestPaginate_LastPage(t *testing.T) {
	data := mkItems(250)
	page, err := Paginate(data, Request{Limit: 100, Offset: 200}, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.Returned != 50 {
		t.Fatalf("returned=%d", page.Meta.Returned)
	}
	if page.Meta.HasMore {
		t.Fatal("has_more should be false on last page")
	}
}

func TestPaginate_OffsetPastEnd(t *testing.T) {
	data := mkItems(10)
	page, err := Paginate(data, Request{Limit: 100, Offset: 999}, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.Returned != 0 {
		t.Fatalf("returned=%d", page.Meta.Returned)
	}
	if page.Meta.HasMore {
		t.Fatal("has_more should be false")
	}
	if page.Meta.Total != 10 {
		t.Fatalf("total=%d", page.Meta.Total)
	}
}

func TestPaginate_StableSortByName(t *testing.T) {
	data := mkItems(50)
	page1, err := Paginate(append([]item(nil), data...), Request{Limit: 10, Offset: 0}, "name")
	if err != nil {
		t.Fatal(err)
	}
	page2, err := Paginate(append([]item(nil), data...), Request{Limit: 10, Offset: 0}, "name")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(page1.Items, page2.Items) {
		t.Fatalf("two paginations of same data should match, got %v vs %v", page1.Items, page2.Items)
	}
	first := page1.Items.([]item)
	for i := 1; i < len(first); i++ {
		if first[i-1].Name > first[i].Name {
			t.Fatalf("not sorted: %s > %s at index %d", first[i-1].Name, first[i].Name, i)
		}
	}
}

func TestPaginate_SortBadFieldFails(t *testing.T) {
	data := mkItems(5)
	_, err := Paginate(data, Request{Limit: 10}, "no_such_field")
	if err == nil {
		t.Fatal("expected error for bad sort key")
	}
}

func TestPaginate_PassThroughStruct(t *testing.T) {
	type stats struct{ Count int }
	in := &stats{Count: 42}
	page, err := Paginate(in, Request{Limit: 10}, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Paginated {
		t.Fatal("non-slice should pass through")
	}
	if page.Items != in {
		t.Fatal("input should be returned unchanged")
	}
}

func TestPaginate_PassThroughNil(t *testing.T) {
	var ptr *[]item
	page, err := Paginate(ptr, Request{Limit: 10}, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Paginated {
		t.Fatal("nil pointer should pass through unpaginated")
	}
}

func TestPaginate_EmptySlice(t *testing.T) {
	data := []item{}
	page, err := Paginate(data, Request{Limit: 10}, "name")
	if err != nil {
		t.Fatal(err)
	}
	if !page.Paginated {
		t.Fatal("empty slice is still a paginated shape")
	}
	if page.Meta.Total != 0 || page.Meta.Returned != 0 || page.Meta.HasMore {
		t.Fatalf("unexpected meta: %+v", page.Meta)
	}
}

func TestPaginate_SortByIntField(t *testing.T) {
	data := []item{{Score: 3}, {Score: 1}, {Score: 2}}
	page, err := Paginate(data, Request{Limit: 10, Offset: 0}, "score")
	if err != nil {
		t.Fatal(err)
	}
	got := page.Items.([]item)
	if got[0].Score != 1 || got[1].Score != 2 || got[2].Score != 3 {
		t.Fatalf("not sorted ascending by score: %+v", got)
	}
}

type sortable interface {
	GetID() string
}

type a struct{ ID string }

func (x *a) GetID() string { return x.ID }

type b struct{ ID string }

func (x *b) GetID() string { return x.ID }

func TestPaginate_SliceOfInterface_SortsByDynamicType(t *testing.T) {
	data := []sortable{&a{ID: "z"}, &b{ID: "a"}, &a{ID: "m"}}
	page, err := Paginate(data, Request{Limit: 10, Offset: 0}, "ID")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	got := page.Items.([]sortable)
	if got[0].GetID() != "a" || got[1].GetID() != "m" || got[2].GetID() != "z" {
		t.Fatalf("not sorted: %v %v %v", got[0].GetID(), got[1].GetID(), got[2].GetID())
	}
}
