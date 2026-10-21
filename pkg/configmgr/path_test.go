package configmgr

import (
	"reflect"
	"testing"
)

func TestConvertValueStringToSlice(t *testing.T) {
	got, err := convertValue(
		"10.250.0.1/30",
		reflect.TypeOf([]string{}),
	)
	if err != nil {
		t.Fatalf("convertValue() returned error: %v", err)
	}

	want := []string{"10.250.0.1/30"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("convertValue() = %#v, want %#v", got, want)
	}
}

func TestConvertValueSliceToSlice(t *testing.T) {
	input := []interface{}{
		"10.250.0.1/30",
		"10.250.0.5/30",
	}

	got, err := convertValue(input, reflect.TypeOf([]string{}))
	if err != nil {
		t.Fatalf("convertValue() returned error: %v", err)
	}

	want := []string{
		"10.250.0.1/30",
		"10.250.0.5/30",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("convertValue() = %#v, want %#v", got, want)
	}
}
