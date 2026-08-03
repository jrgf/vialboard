package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHealthcheckAcceptsReadyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VIAL_HTTP_PORT", port)
	if err := healthcheck(); err != nil {
		t.Fatal(err)
	}
}

func TestDeploymentEnvironment(t *testing.T) {
	t.Setenv("VIAL_AUTO_MIGRATE", "")
	if autoMigrate() {
		t.Fatal("automatic migrations must default off")
	}
	t.Setenv("VIAL_AUTO_MIGRATE", " TRUE ")
	if !autoMigrate() {
		t.Fatal("automatic migrations were not enabled")
	}
	t.Setenv("VIAL_TRUSTED_PROXIES", " 127.0.0.1/32, ,10.0.0.0/8 ")
	if got, want := trustedProxies(), []string{"127.0.0.1/32", "10.0.0.0/8"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("trusted proxies = %v, want %v", got, want)
	}
}
