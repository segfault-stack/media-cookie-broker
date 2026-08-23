package providers

import (
	"reflect"
	"testing"
)

func TestRegistry(t *testing.T) {
	wantIDs := []string{"instagram", "tiktok", "x", "youtube"}
	if got := IDs(); !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("IDs() = %v, want %v", got, wantIDs)
	}

	youtube, ok := Lookup("youtube")
	if !ok {
		t.Fatal("youtube provider is missing")
	}
	if want := []string{"youtube.com"}; !reflect.DeepEqual(youtube.AllowedDomains, want) {
		t.Fatalf("YouTube domains = %v, want %v", youtube.AllowedDomains, want)
	}

	youtube.AllowedDomains[0] = "example.invalid"
	unchanged, _ := Lookup("youtube")
	if unchanged.AllowedDomains[0] != "youtube.com" {
		t.Fatal("Lookup returned mutable registry state")
	}
}
