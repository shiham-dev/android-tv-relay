package config

import "testing"

func TestWithPort(t *testing.T) {
	cases := map[string]string{
		"192.168.1.5":       "192.168.1.5:5555",
		"192.168.1.5:5037":  "192.168.1.5:5037",
		"tv.local":          "tv.local:5555",
		"[2001:db8::1]":     "[2001:db8::1]:5555",
		"[2001:db8::1]:555": "[2001:db8::1]:555",
		"":                  "",
	}
	for in, want := range cases {
		if got := withPort(in); got != want {
			t.Errorf("withPort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("TVS", "living=192.168.1.50, hall=192.168.1.51:5037")
	t.Setenv("HTTP_ADDR", ":9090")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if len(cfg.TVs) != 2 {
		t.Fatalf("want 2 TVs, got %d", len(cfg.TVs))
	}
	if cfg.TVs[0].Name != "living" || cfg.TVs[0].Addr != "192.168.1.50:5555" {
		t.Errorf("TV0 = %+v", cfg.TVs[0])
	}
	if cfg.TVs[1].Addr != "192.168.1.51:5037" {
		t.Errorf("TV1 addr = %q", cfg.TVs[1].Addr)
	}
}

func TestLoadNoTVs(t *testing.T) {
	if _, err := Load(""); err == nil {
		t.Error("expected error with no TVs configured")
	}
}
