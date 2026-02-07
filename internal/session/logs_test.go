package session

import (
	"reflect"
	"testing"
)

func TestRingBufferLast(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Append("a")
	rb.Append("b")
	rb.Append("c")
	rb.Append("d")

	got := rb.Last(3)
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Last(3) = %v, want %v", got, want)
	}
}

func TestRingBufferLastBounds(t *testing.T) {
	rb := NewRingBuffer(2)
	rb.Append("x")

	got := rb.Last(10)
	want := []string{"x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Last(10) = %v, want %v", got, want)
	}

	if gotNil := rb.Last(0); gotNil != nil {
		t.Fatalf("Last(0) = %v, want nil", gotNil)
	}
}
