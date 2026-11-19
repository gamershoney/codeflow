package workspace

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

const (
	defaultMCPReadScope  = "codeflow:read"
	defaultMCPWriteScope = "codeflow:write"
	jwksMaxBytes         = 1 << 20
)

var (
	errSigningKeyNotFound = errors.New("token signing key not found")
	errJWKSUnavailable    = errors.New("JWKS unavailable")
)

type mcpAuthSettings struct {
	readToken    string
	writeToken   string
	readScope    string
	writeScope   string
	resource     string
	issuer       string
	audience     string
	metadataURL  string
	oauthEnabled bool
	jwtVerifier  *oauthJWTVerifier
	metadata     *oauthex.ProtectedResourceMetadata
}

func loadMCPAuthSettings() (mcpAuthSettings, error) {
	settings := mcpAuthSettings{
		readToken:  strings.TrimSpace(os.Getenv("CODEFLOW_MCP_READ_TOKEN")),
		writeToken: strings.TrimSpace(os.Getenv("CODEFLOW_MCP_WRITE_TOKEN")),
		readScope:  fallback(strings.TrimSpace(os.Getenv("CODEFLOW_MCP_OAUTH_READ_SCOPE")), defaultMCPReadScope),
		writeScope: fallback(strings.TrimSpace(os.Getenv("CODEFLOW_MCP_OAUTH_WRITE_SCOPE")), defaultMCPWriteScope),
	}
	resource := strings.TrimSpace(os.Getenv("CODEFLOW_MCP_RESOURCE"))
	issuer := strings.TrimSpace(os.Getenv("CODEFLOW_MCP_OAUTH_ISSUER"))
	jwksURL := strings.TrimSpace(os.Getenv("CODEFLOW_MCP_OAUTH_JWKS_URL"))
	audience := strings.TrimSpace(os.Getenv("CODEFLOW_MCP_OAUTH_AUDIENCE"))
	settings.oauthEnabled = resource != "" || issuer != "" || jwksURL != "" || audience != ""
	if !settings.oauthEnabled {
		return settings, nil
	}
	if resource == "" || issuer == "" || jwksURL == "" {
		return mcpAuthSettings{}, errors.New("CODEFLOW_MCP_RESOURCE, CODEFLOW_MCP_OAUTH_ISSUER, and CODEFLOW_MCP_OAUTH_JWKS_URL must all be set for OAuth")
	}
	if settings.readScope == settings.writeScope {
		return mcpAuthSettings{}, errors.New("OAuth read and write scopes must be different")
	}
	if err := validateOAuthURL("CODEFLOW_MCP_RESOURCE", resource); err != nil {
		return mcpAuthSettings{}, err
	}
	if err := validateOAuthURL("CODEFLOW_MCP_OAUTH_ISSUER", issuer); err != nil {
		return mcpAuthSettings{}, err
	}
	if err := validateOAuthURL("CODEFLOW_MCP_OAUTH_JWKS_URL", jwksURL); err != nil {
		return mcpAuthSettings{}, err
	}
	if audience == "" {
		audience = resource
	}
	settings.resource = resource
	settings.issuer = issuer
	settings.audience = audience
	settings.metadataURL = strings.TrimRight(resource, "/") + "/.well-known/oauth-protected-resource"
	settings.jwtVerifier = &oauthJWTVerifier{
		issuer: issuer, audience: audience,
		keys: &jwksCache{url: jwksURL, client: &http.Client{Timeout: 5 * time.Second}, ttl: 15 * time.Minute},
	}
	settings.metadata = &oauthex.ProtectedResourceMetadata{
		Resource: resource, AuthorizationServers: []string{issuer},
		ScopesSupported: []string{settings.readScope, settings.writeScope}, BearerMethodsSupported: []string{"header"}, ResourceName: "CodeFlow architecture workspace",
	}
	return settings, nil
}

func validateOAuthURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return fmt.Errorf("%s must be an absolute URL", name)
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%s must not contain credentials or a fragment", name)
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if parsed.Scheme == "http" && (strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()) {
		return nil
	}
	return fmt.Errorf("%s must use HTTPS except on loopback", name)
}

func (s mcpAuthSettings) enabled() bool {
	return s.readToken != "" || s.writeToken != "" || s.oauthEnabled
}

func (s mcpAuthSettings) metadataHandler() http.Handler {
	if s.metadata == nil {
		return nil
	}
	return auth.ProtectedResourceMetadataHandler(s.metadata)
}

func (s mcpAuthSettings) toolSecurityMeta(write bool) mcp.Meta {
	if !s.oauthEnabled {
		return nil
	}
	scopes := []string{s.readScope}
	if write {
		scopes = append(scopes, s.writeScope)
	}
	return mcp.Meta{"securitySchemes": []any{map[string]any{"type": "oauth2", "scopes": scopes}}}
}

func (s mcpAuthSettings) verify(ctx context.Context, token string, request *http.Request) (*auth.TokenInfo, error) {
	switch {
	case s.writeToken != "" && secureEqual(token, s.writeToken):
		return &auth.TokenInfo{Scopes: []string{s.readScope, s.writeScope}, UserID: "local-write-token"}, nil
	case s.readToken != "" && secureEqual(token, s.readToken):
		return &auth.TokenInfo{Scopes: []string{s.readScope}, UserID: "local-read-token"}, nil
	case s.jwtVerifier != nil:
		return s.jwtVerifier.verify(ctx, token, request)
	default:
		return nil, auth.ErrInvalidToken
	}
}

func authorizeMCP(next http.Handler, settings mcpAuthSettings) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !settings.enabled() {
			next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), roleContextKey{}, roleWrite)))
			return
		}
		fields := strings.Fields(request.Header.Get("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
			writeAuthChallenge(w, settings, "")
			writeError(w, http.StatusUnauthorized, "MCP authentication required")
			return
		}
		info, err := settings.verify(request.Context(), fields[1], request)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				writeAuthChallenge(w, settings, "invalid_token")
				writeError(w, http.StatusUnauthorized, "invalid or expired MCP access token")
				return
			}
			writeError(w, http.StatusServiceUnavailable, "MCP identity provider is temporarily unavailable")
			return
		}
		hasRead := slices.Contains(info.Scopes, settings.readScope)
		hasWrite := slices.Contains(info.Scopes, settings.writeScope)
		role := roleRead
		if hasRead && hasWrite {
			role = roleWrite
		} else if !hasRead {
			writeAuthChallenge(w, settings, "insufficient_scope")
			writeError(w, http.StatusForbidden, "MCP token lacks the required CodeFlow scope")
			return
		}
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), roleContextKey{}, role)))
	})
}

func writeAuthChallenge(w http.ResponseWriter, settings mcpAuthSettings, oauthError string) {
	if settings.oauthEnabled {
		challenge := fmt.Sprintf(`Bearer resource_metadata=%q, scope=%q`, settings.metadataURL, settings.readScope+" "+settings.writeScope)
		if oauthError != "" {
			challenge += fmt.Sprintf(`, error=%q`, oauthError)
		}
		w.Header().Set("WWW-Authenticate", challenge)
		return
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="codeflow-mcp"`)
}

func secureEqual(got, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

type oauthJWTClaims struct {
	jwt.RegisteredClaims
	Scope       string   `json:"scope,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	SCP         any      `json:"scp,omitempty"`
}

type oauthJWTVerifier struct {
	issuer   string
	audience string
	keys     *jwksCache
}

func (v *oauthJWTVerifier) verify(ctx context.Context, rawToken string, _ *http.Request) (*auth.TokenInfo, error) {
	claims := &oauthJWTClaims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errSigningKeyNotFound
		}
		key, keyAlg, err := v.keys.key(ctx, kid)
		if err != nil {
			return nil, err
		}
		if keyAlg != "" && keyAlg != token.Method.Alg() {
			return nil, errSigningKeyNotFound
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}), jwt.WithIssuer(v.issuer), jwt.WithAudience(v.audience), jwt.WithExpirationRequired(), jwt.WithLeeway(30*time.Second))
	if err != nil || token == nil || !token.Valid {
		if errors.Is(err, errJWKSUnavailable) {
			return nil, err
		}
		return nil, auth.ErrInvalidToken
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, auth.ErrInvalidToken
	}
	scopes := splitTokenScopes(claims)
	return &auth.TokenInfo{Scopes: scopes, Expiration: claims.ExpiresAt.Time, UserID: claims.Subject, Extra: map[string]any{"issuer": claims.Issuer}}, nil
}

func splitTokenScopes(claims *oauthJWTClaims) []string {
	values := append([]string{}, strings.Fields(claims.Scope)...)
	values = append(values, claims.Permissions...)
	switch scoped := claims.SCP.(type) {
	case string:
		values = append(values, strings.Fields(scoped)...)
	case []any:
		for _, value := range scoped {
			if text, ok := value.(string); ok {
				values = append(values, text)
			}
		}
	}
	unique := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			unique = append(unique, value)
		}
	}
	return unique
}

type cachedJWK struct {
	key any
	alg string
}

type jwksCache struct {
	url             string
	client          *http.Client
	ttl             time.Duration
	mu              sync.Mutex
	keys            map[string]cachedJWK
	fetchedAt       time.Time
	lastMissRefresh time.Time
}

func (c *jwksCache) key(ctx context.Context, kid string) (any, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.keys == nil || time.Since(c.fetchedAt) >= c.ttl {
		if err := c.refresh(ctx); err != nil {
			return nil, "", err
		}
	}
	if key, ok := c.keys[kid]; ok {
		return key.key, key.alg, nil
	}
	if time.Since(c.lastMissRefresh) >= time.Minute {
		c.lastMissRefresh = time.Now()
		if err := c.refresh(ctx); err != nil {
			return nil, "", err
		}
		if key, ok := c.keys[kid]; ok {
			return key.key, key.alg, nil
		}
	}
	return nil, "", errSigningKeyNotFound
}

func (c *jwksCache) refresh(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", errJWKSUnavailable, err)
	}
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", errJWKSUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", errJWKSUnavailable, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, jwksMaxBytes+1))
	if err != nil || len(data) > jwksMaxBytes {
		return fmt.Errorf("%w: invalid response size", errJWKSUnavailable)
	}
	var document struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("%w: invalid JSON", errJWKSUnavailable)
	}
	parsed := map[string]cachedJWK{}
	for _, raw := range document.Keys {
		kid, key, alg, err := parseJWK(raw)
		if err == nil && kid != "" {
			parsed[kid] = cachedJWK{key: key, alg: alg}
		}
	}
	if len(parsed) == 0 {
		return fmt.Errorf("%w: no supported signing keys", errJWKSUnavailable)
	}
	c.keys, c.fetchedAt = parsed, time.Now()
	return nil
}

func parseJWK(raw json.RawMessage) (string, any, string, error) {
	var key struct {
		KTY string `json:"kty"`
		KID string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
		CRV string `json:"crv"`
		X   string `json:"x"`
		Y   string `json:"y"`
	}
	if err := json.Unmarshal(raw, &key); err != nil || key.KID == "" || (key.Use != "" && key.Use != "sig") {
		return "", nil, "", errors.New("unsupported JWK")
	}
	switch key.KTY {
	case "RSA":
		n, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			return "", nil, "", err
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil || len(eBytes) == 0 || len(eBytes) > 4 {
			return "", nil, "", errors.New("invalid RSA exponent")
		}
		exponent := 0
		for _, value := range eBytes {
			exponent = exponent<<8 | int(value)
		}
		if exponent < 3 {
			return "", nil, "", errors.New("invalid RSA exponent")
		}
		return key.KID, &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exponent}, key.Alg, nil
	case "EC":
		curves := map[string]elliptic.Curve{"P-256": elliptic.P256(), "P-384": elliptic.P384(), "P-521": elliptic.P521()}
		curve := curves[key.CRV]
		x, xErr := base64.RawURLEncoding.DecodeString(key.X)
		y, yErr := base64.RawURLEncoding.DecodeString(key.Y)
		if curve == nil || xErr != nil || yErr != nil {
			return "", nil, "", errors.New("invalid EC key")
		}
		publicKey := &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !curve.IsOnCurve(publicKey.X, publicKey.Y) {
			return "", nil, "", errors.New("EC key is not on curve")
		}
		return key.KID, publicKey, key.Alg, nil
	default:
		return "", nil, "", errors.New("unsupported JWK type")
	}
}
