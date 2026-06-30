package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_MissingRequired(t *testing.T) {
	c := &Config{}
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "C5_DB_DSN")
	assert.Contains(t, err.Error(), "C5_REDIS_ADDR")
	assert.Contains(t, err.Error(), "C5_JWT_SECRET")
}

func TestValidate_OKWhenRequiredPresent(t *testing.T) {
	c := &Config{}
	c.DB.DSN = "postgres://localhost/c5"
	c.Redis.Addr = "localhost:6379"
	c.JWT.Secret = "a-sufficiently-long-test-secret-0123456789"
	assert.NoError(t, c.Validate())
}

func TestValidate_ShortSecretRejected(t *testing.T) {
	c := &Config{}
	c.DB.DSN = "postgres://localhost/c5"
	c.Redis.Addr = "localhost:6379"
	c.JWT.Secret = "too-short"
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestLoad_FromEnv(t *testing.T) {
	t.Setenv("C5_DB_DSN", "postgres://u:p@localhost:5432/c5")
	t.Setenv("C5_REDIS_ADDR", "localhost:6379")
	t.Setenv("C5_JWT_SECRET", "a-sufficiently-long-test-secret-0123456789")
	t.Setenv("C5_SERVER_PORT", "9099")
	t.Setenv("C5_JWT_ACCESS_TTL", "10m")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "postgres://u:p@localhost:5432/c5", cfg.DB.DSN)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, "a-sufficiently-long-test-secret-0123456789", cfg.JWT.Secret)
	assert.Equal(t, 9099, cfg.Server.Port)
	assert.Equal(t, 10*time.Minute, cfg.JWT.AccessTTL)

	// defaults applied
	assert.Equal(t, "c5-api", cfg.JWT.Issuer)
	assert.Equal(t, "ap-guangzhou", cfg.COS.Region)
	assert.Equal(t, 720*time.Hour, cfg.JWT.RefreshTTL)
}

func TestLoad_TrustedProxies_ParsedFromCSV(t *testing.T) {
	t.Setenv("C5_DB_DSN", "postgres://u:p@localhost:5432/c5")
	t.Setenv("C5_REDIS_ADDR", "localhost:6379")
	t.Setenv("C5_JWT_SECRET", "a-sufficiently-long-test-secret-0123456789")
	t.Setenv("C5_SERVER_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.1")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.0/8", "192.168.1.1"}, cfg.Server.TrustedProxies)
}

func TestValidate_BadTrustedProxyRejected(t *testing.T) {
	c := &Config{}
	c.DB.DSN = "postgres://localhost/c5"
	c.Redis.Addr = "localhost:6379"
	c.JWT.Secret = "a-sufficiently-long-test-secret-0123456789"
	c.Server.TrustedProxies = []string{"not-an-ip"}
	err := c.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TRUSTED_PROXIES")
}

func TestLoad_FailFastWhenMissing(t *testing.T) {
	// No required env vars set in this subprocess-free test environment.
	t.Setenv("C5_DB_DSN", "")
	t.Setenv("C5_REDIS_ADDR", "")
	t.Setenv("C5_JWT_SECRET", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required configuration")
}

// corsBase returns a prod config with the always-required secrets present, so
// only the CORS-origin rules are under test.
func corsBase() *Config {
	c := &Config{Env: "prod"}
	c.DB.DSN = "postgres://localhost/c5"
	c.Redis.Addr = "localhost:6379"
	c.JWT.Secret = "a-sufficiently-long-test-secret-0123456789"
	return c
}

func TestValidate_CORSAllowedOrigins_ProdEmptyRejected(t *testing.T) {
	c := corsBase()
	err := c.ValidateServerCORS()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "C5_CORS_ALLOWED_ORIGINS")
}

// Regression: worker / migrate / create-admin share config.Load()/Validate() but have
// no HTTP server; prod must NOT require CORS for them (only cmd/api calls
// ValidateServerCORS). Guards against the bug where worker crash-looped in prod.
func TestValidate_NoCORSRequirement_ForNonHTTPBinaries(t *testing.T) {
	c := corsBase() // Env=prod, DB/Redis/JWT set, no AllowedOrigins
	assert.NoError(t, c.Validate())
}

func TestValidate_CORSAllowedOrigins_ProdBadRejected(t *testing.T) {
	for _, bad := range []string{
		"*",                       // wildcard
		"https://*.x.com",         // wildcard host
		"http://admin.x.com",      // not https
		"https://admin.x.com/app", // has path
		"https://admin.x.com/",    // trailing slash
		"https://admin.x.com?q=1", // query
		"admin.x.com",             // no scheme
		"https://u@admin.x.com",   // userinfo
		"https://u:p@admin.x.com", // userinfo with password
		"https://admin.x.com#",    // empty trailing fragment
		"https://admin.x.com.",    // trailing-dot FQDN
	} {
		c := corsBase()
		c.Server.AllowedOrigins = []string{bad}
		assert.Errorf(t, c.ValidateServerCORS(), "expected %q to be rejected", bad)
	}
}

func TestValidate_CORSAllowedOrigins_ProdValidOK(t *testing.T) {
	c := corsBase()
	// A non-default port is a legitimate Origin and must be accepted.
	c.Server.AllowedOrigins = []string{"https://admin.x.com", "https://admin.example.cn", "https://admin.x.com:8443"}
	assert.NoError(t, c.ValidateServerCORS())
}

func TestValidate_CORSAllowedOrigins_NonProdEmptyOK(t *testing.T) {
	c := corsBase()
	c.Env = "dev"
	c.Server.AllowedOrigins = nil
	assert.NoError(t, c.ValidateServerCORS())
}

func TestLoad_CORSAllowedOrigins_ParsedFromCSV(t *testing.T) {
	t.Setenv("C5_DB_DSN", "postgres://u:p@localhost:5432/c5")
	t.Setenv("C5_REDIS_ADDR", "localhost:6379")
	t.Setenv("C5_JWT_SECRET", "a-sufficiently-long-test-secret-0123456789")
	t.Setenv("C5_CORS_ALLOWED_ORIGINS", "https://admin.x.com, https://admin2.x.com")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"https://admin.x.com", "https://admin2.x.com"}, cfg.Server.AllowedOrigins)
}
