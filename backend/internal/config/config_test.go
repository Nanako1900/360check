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
