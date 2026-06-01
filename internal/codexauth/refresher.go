// Package codexauth implements a one-shot refresher for Codex (OpenAI)
// OAuth credentials stored in Kubernetes Secrets.
//
// Codex ChatGPT-mode authentication is a JSON bundle (the contents of
// ~/.codex/auth.json) carrying a short-lived access_token and id_token plus a
// long-lived refresh_token. Kelos injects this bundle into agent pods from the
// CODEX_AUTH_JSON key of a credentials Secret (see
// internal/controller/job_builder.go). Agent pods are ephemeral and never
// persist a refreshed bundle back, so if no task runs for longer than the
// access token's lifetime the credential goes stale; if no task runs for
// longer than the refresh token's idle lifetime the bundle can no longer be
// refreshed at all.
//
// This package refreshes the bundle independently of agent activity. It is
// meant to be run on a schedule (a CronJob) by the controller's ServiceAccount,
// which already holds get/list/update on Secrets. It performs the OAuth2
// refresh_token grant directly so there is nothing to "trigger" — it does not
// depend on the Codex CLI being present or on the CLI's lazy refresh heuristics.
package codexauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// RefreshLabel is the Secret label that opts a credentials Secret into
	// scheduled Codex OAuth refresh. Only Secrets carrying RefreshLabel set to
	// "true" are considered.
	RefreshLabel = "kelos.dev/codex-oauth-refresh"

	// secretKey is the Secret data key holding the Codex auth.json bundle. It
	// matches the env var injected into Codex agent pods.
	secretKey = "CODEX_AUTH_JSON"

	// DefaultTokenEndpoint is the OAuth2 token endpoint used by the Codex CLI
	// to refresh ChatGPT-mode credentials. It is overridable so the value can
	// be corrected without a code change if OpenAI relocates the endpoint.
	DefaultTokenEndpoint = "https://auth.openai.com/oauth/token"

	// DefaultClientID is the OAuth2 client_id used by the Codex CLI. It is
	// overridable for the same reason as DefaultTokenEndpoint.
	DefaultClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

	// refreshScope is the OAuth2 scope requested on refresh, matching the
	// Codex CLI.
	refreshScope = "openid profile email"

	// originator identifies the client to OpenAI, mirroring the Codex CLI.
	// auth.openai.com sits behind a WAF that can challenge unrecognized
	// clients, so the refresh request presents the same identity as the CLI.
	originator = "codex_cli_rs"

	// userAgent is sent on the refresh request. Go's default User-Agent
	// (Go-http-client/...) is liable to be challenged by the endpoint's WAF;
	// presenting the Codex CLI's identity avoids that.
	userAgent = "codex_cli_rs"
)

// Options configures a refresh run.
type Options struct {
	// Namespace limits the Secret scan to a single namespace. Empty means all
	// namespaces (requires cluster-wide list permission on Secrets).
	Namespace string
	// TokenEndpoint is the OAuth2 token endpoint. Defaults to
	// DefaultTokenEndpoint when empty.
	TokenEndpoint string
	// ClientID is the OAuth2 client_id. Defaults to DefaultClientID when empty.
	ClientID string
	// HTTPClient performs the refresh request. Defaults to a client with a
	// 30s timeout when nil.
	HTTPClient *http.Client
	// now returns the current time. Defaults to time.Now. Injectable for tests.
	now func() time.Time
}

func (o *Options) applyDefaults() {
	if o.TokenEndpoint == "" {
		o.TokenEndpoint = DefaultTokenEndpoint
	}
	if o.ClientID == "" {
		o.ClientID = DefaultClientID
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if o.now == nil {
		o.now = time.Now
	}
}

// Run refreshes every opted-in Codex OAuth credentials Secret in scope. Secrets
// without a CODEX_AUTH_JSON bundle, or whose bundle carries no refresh_token
// (e.g. API-key credentials), are skipped. A failure to refresh one Secret does
// not abort the others; Run returns an aggregate error if any Secret failed.
func Run(ctx context.Context, log logr.Logger, clientset kubernetes.Interface, opts Options) error {
	opts.applyDefaults()
	if clientset == nil {
		return errors.New("clientset is required")
	}

	secrets, err := clientset.CoreV1().Secrets(opts.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: RefreshLabel + "=true",
	})
	if err != nil {
		return fmt.Errorf("listing Codex OAuth Secrets: %w", err)
	}

	log.Info("Scanning Codex OAuth credentials Secrets", "count", len(secrets.Items))

	var refreshed, skipped int
	var failures []error
	for i := range secrets.Items {
		s := &secrets.Items[i]
		changed, err := refreshSecret(ctx, log, clientset, s, opts)
		switch {
		case err != nil:
			// Name the resource so operators can act on the signal; never log
			// the token bundle itself.
			log.Error(err, "Failed to refresh Codex OAuth Secret", "namespace", s.Namespace, "name", s.Name)
			failures = append(failures, fmt.Errorf("secret %s/%s: %w", s.Namespace, s.Name, err))
		case changed:
			refreshed++
		default:
			skipped++
		}
	}

	log.Info("Codex OAuth refresh complete", "refreshed", refreshed, "skipped", skipped, "failed", len(failures))
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d Codex OAuth Secret(s) failed to refresh: %w", len(failures), len(secrets.Items), errors.Join(failures...))
	}
	return nil
}

// refreshSecret refreshes a single Secret's bundle. It returns true when the
// bundle changed and was written back, false when the Secret was a no-op
// (no bundle, or no refresh_token).
func refreshSecret(ctx context.Context, log logr.Logger, clientset kubernetes.Interface, s *corev1.Secret, opts Options) (bool, error) {
	raw, ok := s.Data[secretKey]
	if !ok || len(bytes.TrimSpace(raw)) == 0 {
		// Not a Codex OAuth Secret (e.g. API-key credentials live under a
		// different key). Nothing to refresh.
		return false, nil
	}

	updated, changed, err := refreshBundle(ctx, raw, opts)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	// Write back via read-modify-Update on the Secret already held from the
	// List. Mutating only the CODEX_AUTH_JSON key preserves the other keys,
	// and Update (rather than Patch) matches the verbs the controller
	// ServiceAccount is granted (get/list/update); it has no patch verb.
	if s.Data == nil {
		s.Data = map[string][]byte{}
	}
	s.Data[secretKey] = updated
	if _, err := clientset.CoreV1().Secrets(s.Namespace).Update(ctx, s, metav1.UpdateOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf("secret no longer exists: %w", err)
		}
		return false, fmt.Errorf("updating Secret: %w", err)
	}

	log.Info("Refreshed Codex OAuth credential", "namespace", s.Namespace, "name", s.Name)
	return true, nil
}

// tokens is the OAuth token sub-object of a Codex auth.json bundle.
type tokenResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// refreshBundle performs the OAuth2 refresh_token grant and returns the updated
// auth.json bytes. The bundle is parsed into a generic map so unknown fields
// (e.g. OPENAI_API_KEY, tokens.account_id) are preserved verbatim. changed is
// false when the bundle has no refresh_token to exchange.
func refreshBundle(ctx context.Context, raw []byte, opts Options) (updated []byte, changed bool, err error) {
	var bundle map[string]any
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return nil, false, fmt.Errorf("parsing auth.json bundle: %w", err)
	}

	tokens, ok := bundle["tokens"].(map[string]any)
	if !ok {
		// API-key style bundle (no OAuth tokens object): nothing to refresh.
		return nil, false, nil
	}
	refreshToken, _ := tokens["refresh_token"].(string)
	if refreshToken == "" {
		return nil, false, nil
	}

	resp, err := exchange(ctx, opts, refreshToken)
	if err != nil {
		return nil, false, err
	}

	// Update only the fields the grant returns; OpenAI may omit a rotated
	// refresh_token, in which case the existing one is retained.
	tokens["access_token"] = resp.AccessToken
	tokens["id_token"] = resp.IDToken
	if resp.RefreshToken != "" {
		tokens["refresh_token"] = resp.RefreshToken
	}
	bundle["tokens"] = tokens
	bundle["last_refresh"] = opts.now().UTC().Format(time.RFC3339)

	out, err := json.Marshal(bundle)
	if err != nil {
		return nil, false, fmt.Errorf("serializing refreshed bundle: %w", err)
	}
	return out, true, nil
}

// exchange performs the OAuth2 refresh_token grant against the token endpoint.
func exchange(ctx context.Context, opts Options, refreshToken string) (*tokenResponse, error) {
	body, err := json.Marshal(map[string]string{
		"client_id":     opts.ClientID,
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"scope":         refreshScope,
	})
	if err != nil {
		return nil, fmt.Errorf("building refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, opts.TokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("originator", originator)

	res, err := opts.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer res.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		// Do not include the response body: it may echo token material.
		return nil, fmt.Errorf("refresh endpoint returned status %d", res.StatusCode)
	}

	var resp tokenResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, fmt.Errorf("parsing refresh response: %w", err)
	}
	if resp.AccessToken == "" {
		return nil, errors.New("refresh response missing access_token")
	}
	return &resp, nil
}
