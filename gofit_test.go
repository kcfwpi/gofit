package gofit

import (
	"testing"
)

func TestNewFIT(t *testing.T) {
	fit := NewFIT("test.fit")
	go fit.Parse()

	for m := range fit.MessageChan {
		t.Logf("%+v\n", m.Fields)
	}
}
