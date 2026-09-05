package get_test

import (
	"context"
	get "github.com/ahillspace/tadx/actions/version/get"
	"testing"
	"time"
)

type current string

func (c current) Current() string { return string(c) }

type checker struct{ calls int }

func (c *checker) Latest(context.Context) (get.Release, error) {
	c.calls++
	return get.Release{Version: "1.2.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v1.2.0", PublishedAt: time.Now()}, nil
}
func TestOfflineByDefault(t *testing.T) {
	c := &checker{}
	out, err := get.New(current("1.1.0"), c).Execute(context.Background(), get.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 0 || out.Version != "1.1.0" || out.Status != "installed" {
		t.Fatalf("out=%#v calls=%d", out, c.calls)
	}
}

func TestCheckReportsCurrentVersion(t *testing.T) {
	c := &checker{}
	out, err := get.New(current("1.2.0"), c).Execute(context.Background(), get.Input{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || out.UpdateAvailable || out.Status != "current" {
		t.Fatalf("out=%#v calls=%d", out, c.calls)
	}
}
func TestCheckReportsUpdate(t *testing.T) {
	c := &checker{}
	out, err := get.New(current("1.1.0"), c).Execute(context.Background(), get.Input{Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.calls != 1 || !out.UpdateAvailable || out.Status != "update-available" {
		t.Fatalf("out=%#v calls=%d", out, c.calls)
	}
}
