package codexauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

// fixedNow returns a stable timestamp for deterministic assertions.
func fixedNow() time.Time {
	return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
}

// newTokenServer returns a test server that issues the given tokens for a
// refresh_token grant and records the requests it received.
func newTokenServer(t *testing.T, access, id, refresh string) (*httptest.Server, *[]map[string]string) {
	t.Helper()
	var got []map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The request must present the Codex CLI's identity so the endpoint's
		// WAF does not challenge it.
		if ua := r.Header.Get("User-Agent"); ua != userAgent {
			t.Errorf("expected User-Agent %q, got %q", userAgent, ua)
		}
		if o := r.Header.Get("originator"); o != originator {
			t.Errorf("expected originator %q, got %q", originator, o)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		got = append(got, body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  access,
			IDToken:      id,
			RefreshToken: refresh,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func oauthBundle(t *testing.T, access, id, refresh string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"access_token":  access,
			"id_token":      id,
			"refresh_token": refresh,
			"account_id":    "acct_123",
		},
		"last_refresh": "2026-05-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func labeledSecret(name string, data map[string]string) *corev1.Secret {
	d := map[string][]byte{}
	for k, v := range data {
		d[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    map[string]string{RefreshLabel: "true"},
		},
		Data: d,
	}
}

func TestRun_RefreshesOAuthSecret(t *testing.T) {
	srv, got := newTokenServer(t, "new-access", "new-id", "new-refresh")
	secret := labeledSecret("codex-creds", map[string]string{
		secretKey: oauthBundle(t, "old-access", "old-id", "old-refresh"),
	})
	cs := fakeclientset.NewSimpleClientset(secret)

	err := Run(context.Background(), logr.Discard(), cs, Options{
		TokenEndpoint: srv.URL,
		now:           fixedNow,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// The refresh grant must carry the existing refresh_token.
	if len(*got) != 1 {
		t.Fatalf("expected 1 refresh request, got %d", len(*got))
	}
	if (*got)[0]["grant_type"] != "refresh_token" {
		t.Errorf("expected grant_type refresh_token, got %q", (*got)[0]["grant_type"])
	}
	if (*got)[0]["refresh_token"] != "old-refresh" {
		t.Errorf("expected old refresh_token sent, got %q", (*got)[0]["refresh_token"])
	}

	updated, err := cs.CoreV1().Secrets("default").Get(context.Background(), "codex-creds", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var bundle map[string]any
	if err := json.Unmarshal(updated.Data[secretKey], &bundle); err != nil {
		t.Fatalf("parsing updated bundle: %v", err)
	}
	tokens := bundle["tokens"].(map[string]any)
	if tokens["access_token"] != "new-access" {
		t.Errorf("expected refreshed access_token, got %q", tokens["access_token"])
	}
	if tokens["id_token"] != "new-id" {
		t.Errorf("expected refreshed id_token, got %q", tokens["id_token"])
	}
	if tokens["refresh_token"] != "new-refresh" {
		t.Errorf("expected rotated refresh_token, got %q", tokens["refresh_token"])
	}
	// Unknown fields must be preserved.
	if tokens["account_id"] != "acct_123" {
		t.Errorf("expected account_id preserved, got %q", tokens["account_id"])
	}
	if _, ok := bundle["OPENAI_API_KEY"]; !ok {
		t.Error("expected OPENAI_API_KEY field preserved")
	}
	if bundle["last_refresh"] != fixedNow().Format(time.RFC3339) {
		t.Errorf("expected last_refresh updated to now, got %q", bundle["last_refresh"])
	}
}

func TestRun_PreservesOtherSecretKeys(t *testing.T) {
	srv, _ := newTokenServer(t, "new-access", "new-id", "new-refresh")
	secret := labeledSecret("codex-creds", map[string]string{
		secretKey:    oauthBundle(t, "old-access", "old-id", "old-refresh"),
		"OTHER_KEY":  "keep-me",
		"github-app": "stay",
	})
	cs := fakeclientset.NewSimpleClientset(secret)

	if err := Run(context.Background(), logr.Discard(), cs, Options{TokenEndpoint: srv.URL, now: fixedNow}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	updated, err := cs.CoreV1().Secrets("default").Get(context.Background(), "codex-creds", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.Data["OTHER_KEY"]) != "keep-me" {
		t.Errorf("expected OTHER_KEY preserved, got %q", updated.Data["OTHER_KEY"])
	}
	if string(updated.Data["github-app"]) != "stay" {
		t.Errorf("expected github-app preserved, got %q", updated.Data["github-app"])
	}
}

func TestRun_APIKeySecretIsNoOp(t *testing.T) {
	srv, got := newTokenServer(t, "new-access", "new-id", "new-refresh")
	// API-key credentials live under a different key and carry no tokens object.
	secret := labeledSecret("codex-apikey", map[string]string{
		"CODEX_API_KEY": "sk-test",
	})
	cs := fakeclientset.NewSimpleClientset(secret)

	if err := Run(context.Background(), logr.Discard(), cs, Options{TokenEndpoint: srv.URL, now: fixedNow}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("expected no refresh request for api-key secret, got %d", len(*got))
	}
	updated, err := cs.CoreV1().Secrets("default").Get(context.Background(), "codex-apikey", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(updated.Data["CODEX_API_KEY"]) != "sk-test" {
		t.Errorf("expected api-key untouched, got %q", updated.Data["CODEX_API_KEY"])
	}
}

func TestRun_BundleWithoutRefreshTokenIsNoOp(t *testing.T) {
	srv, got := newTokenServer(t, "new-access", "new-id", "new-refresh")
	// A bundle whose tokens object lacks a refresh_token cannot be refreshed.
	bundle, _ := json.Marshal(map[string]any{
		"tokens": map[string]any{"access_token": "only-access"},
	})
	secret := labeledSecret("codex-no-rt", map[string]string{secretKey: string(bundle)})
	cs := fakeclientset.NewSimpleClientset(secret)

	if err := Run(context.Background(), logr.Discard(), cs, Options{TokenEndpoint: srv.URL, now: fixedNow}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("expected no refresh request, got %d", len(*got))
	}
}

func TestRun_RetainsRefreshTokenWhenNotRotated(t *testing.T) {
	// OpenAI may omit a rotated refresh_token; the existing one must be kept.
	srv, _ := newTokenServer(t, "new-access", "new-id", "")
	secret := labeledSecret("codex-creds", map[string]string{
		secretKey: oauthBundle(t, "old-access", "old-id", "old-refresh"),
	})
	cs := fakeclientset.NewSimpleClientset(secret)

	if err := Run(context.Background(), logr.Discard(), cs, Options{TokenEndpoint: srv.URL, now: fixedNow}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	updated, _ := cs.CoreV1().Secrets("default").Get(context.Background(), "codex-creds", metav1.GetOptions{})
	var bundle map[string]any
	_ = json.Unmarshal(updated.Data[secretKey], &bundle)
	tokens := bundle["tokens"].(map[string]any)
	if tokens["refresh_token"] != "old-refresh" {
		t.Errorf("expected refresh_token retained, got %q", tokens["refresh_token"])
	}
	if tokens["access_token"] != "new-access" {
		t.Errorf("expected access_token refreshed, got %q", tokens["access_token"])
	}
}

func TestRun_SkipsUnlabeledSecret(t *testing.T) {
	srv, got := newTokenServer(t, "new-access", "new-id", "new-refresh")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "unlabeled", Namespace: "default"},
		Data:       map[string][]byte{secretKey: []byte(oauthBundle(t, "a", "b", "c"))},
	}
	cs := fakeclientset.NewSimpleClientset(secret)

	if err := Run(context.Background(), logr.Discard(), cs, Options{TokenEndpoint: srv.URL, now: fixedNow}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("expected unlabeled secret to be skipped, got %d refresh requests", len(*got))
	}
}

func TestRun_AggregatesPerSecretFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	secret := labeledSecret("codex-creds", map[string]string{
		secretKey: oauthBundle(t, "old-access", "old-id", "old-refresh"),
	})
	cs := fakeclientset.NewSimpleClientset(secret)

	err := Run(context.Background(), logr.Discard(), cs, Options{TokenEndpoint: srv.URL, now: fixedNow})
	if err == nil {
		t.Fatal("expected an error when a Secret fails to refresh")
	}
	if !strings.Contains(err.Error(), "default/codex-creds") {
		t.Errorf("expected error to name the failing Secret, got %q", err.Error())
	}
	// On failure the bundle must be left untouched (no error body leakage either).
	if strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("error must not echo the endpoint response body, got %q", err.Error())
	}
}

func TestRun_RequiresClientset(t *testing.T) {
	if err := Run(context.Background(), logr.Discard(), nil, Options{}); err == nil {
		t.Fatal("expected an error when clientset is nil")
	}
}
