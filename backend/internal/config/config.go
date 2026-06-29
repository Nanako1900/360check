// Package config loads and validates runtime configuration from environment
// variables (prefix C5_) and an optional config.yaml, failing fast at startup
// when a required secret is missing.
package config

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the fully-resolved application configuration.
type Config struct {
	Env    string
	Server ServerConfig
	DB     DBConfig
	Redis  RedisConfig
	JWT    JWTConfig
	COS    COSConfig
	Observ ObservConfig
}

// ServerConfig controls the HTTP server.
type ServerConfig struct {
	Port            int
	Mode            string // gin mode: debug | release | test
	ShutdownTimeout time.Duration
	// TrustedProxies are the ingress proxy CIDRs/IPs trusted when resolving the
	// client IP from X-Forwarded-For. Empty => trust none (ClientIP is the direct
	// peer), so a client cannot spoof its IP to bypass the per-IP auth rate limit.
	TrustedProxies []string
	// AllowedOrigins is the exact CORS allow-list (browser Origins) permitted to
	// call the API cross-origin. Required (non-empty, https, no wildcard/path) in
	// prod once the SPA is served from a different origin (e.g. EdgeOne Pages).
	AllowedOrigins []string
}

// DBConfig controls the PostgreSQL (PostGIS) connection pool.
type DBConfig struct {
	DSN         string
	MaxConns    int32
	MinConns    int32
	HealthCheck time.Duration
}

// RedisConfig controls the Redis client (refresh tokens / casbin watcher / asynq).
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// JWTConfig controls access-token signing and token lifetimes.
type JWTConfig struct {
	Secret     string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// COSConfig holds Tencent COS / CAM STS settings. Validated lazily when media
// features initialize (P5); not required to boot c5-api or run /healthz.
type COSConfig struct {
	SecretID       string
	SecretKey      string
	AppID          string // Tencent APPID (numeric), required by CAM STS credential scoping
	Region         string
	BucketOriginal string
	BucketWeb      string
	BucketThumb    string
	CDNDomain      string
	STSRoleArn     string
}

// ObservConfig controls observability exporters.
type ObservConfig struct {
	ServiceName  string
	OTLPEndpoint string
}

// Load resolves configuration from env + optional config.yaml, applies defaults
// and validates required secrets (fail-fast).
func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("C5")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Optional config file; environment variables remain authoritative.
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/c5")
	_ = v.ReadInConfig()

	// Keys without a default still need an explicit bind so AutomaticEnv resolves
	// them (viper only auto-resolves keys it already knows about).
	for _, k := range []string{
		"db.dsn", "redis.addr", "redis.password",
		"server.trusted_proxies",
		"cors.allowed_origins",
		"jwt.secret",
		"cos.secret_id", "cos.secret_key", "cos.app_id", "cos.bucket_original",
		"cos.bucket_web", "cos.bucket_thumb", "cos.cdn_domain", "cos.sts_role_arn",
		"observ.otlp_endpoint",
	} {
		_ = v.BindEnv(k)
	}

	v.SetDefault("env", "dev")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "release")
	v.SetDefault("server.shutdown_timeout", "15s")
	v.SetDefault("db.max_conns", 10)
	v.SetDefault("db.min_conns", 2)
	v.SetDefault("db.health_check", "30s")
	v.SetDefault("redis.db", 0)
	v.SetDefault("jwt.issuer", "c5-api")
	v.SetDefault("jwt.access_ttl", "15m")
	v.SetDefault("jwt.refresh_ttl", "720h")
	v.SetDefault("cos.region", "ap-guangzhou")
	v.SetDefault("observ.service_name", "c5-api")

	cfg := &Config{
		Env: v.GetString("env"),
		Server: ServerConfig{
			Port:            v.GetInt("server.port"),
			Mode:            v.GetString("server.mode"),
			ShutdownTimeout: v.GetDuration("server.shutdown_timeout"),
			TrustedProxies:  splitCSV(v.GetString("server.trusted_proxies")),
			AllowedOrigins:  splitCSV(v.GetString("cors.allowed_origins")),
		},
		DB: DBConfig{
			DSN:         v.GetString("db.dsn"),
			MaxConns:    int32(v.GetInt("db.max_conns")),
			MinConns:    int32(v.GetInt("db.min_conns")),
			HealthCheck: v.GetDuration("db.health_check"),
		},
		Redis: RedisConfig{
			Addr:     v.GetString("redis.addr"),
			Password: v.GetString("redis.password"),
			DB:       v.GetInt("redis.db"),
		},
		JWT: JWTConfig{
			Secret:     v.GetString("jwt.secret"),
			Issuer:     v.GetString("jwt.issuer"),
			AccessTTL:  v.GetDuration("jwt.access_ttl"),
			RefreshTTL: v.GetDuration("jwt.refresh_ttl"),
		},
		COS: COSConfig{
			SecretID:       v.GetString("cos.secret_id"),
			SecretKey:      v.GetString("cos.secret_key"),
			AppID:          v.GetString("cos.app_id"),
			Region:         v.GetString("cos.region"),
			BucketOriginal: v.GetString("cos.bucket_original"),
			BucketWeb:      v.GetString("cos.bucket_web"),
			BucketThumb:    v.GetString("cos.bucket_thumb"),
			CDNDomain:      v.GetString("cos.cdn_domain"),
			STSRoleArn:     v.GetString("cos.sts_role_arn"),
		},
		Observ: ObservConfig{
			ServiceName:  v.GetString("observ.service_name"),
			OTLPEndpoint: v.GetString("observ.otlp_endpoint"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate enforces fail-fast presence of the secrets required to boot c5-api.
// DB, Redis and the JWT secret are always required; COS/CAM credentials are
// validated lazily when media features initialize (P5) so local dev and the
// /healthz integration test (PostGIS + Redis only) can boot without them.
//
// P5 TODO (do not drop — docs/01 §P0 line ~409 lists COS in the startup
// fail-fast set): when media init lands, add COS.SecretID/SecretKey/STSRoleArn/
// buckets to this required set (or a dedicated ValidateMedia()) so a missing COS
// secret fails at startup, not at the first media call.
func (c *Config) Validate() error {
	var missing []string
	if c.DB.DSN == "" {
		missing = append(missing, "C5_DB_DSN")
	}
	if c.Redis.Addr == "" {
		missing = append(missing, "C5_REDIS_ADDR")
	}
	if c.JWT.Secret == "" {
		missing = append(missing, "C5_JWT_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
	}
	// HS256 secret strength: a short/low-entropy secret is brute-forceable offline
	// from any issued token. Require >= 32 bytes (256-bit).
	if len(c.JWT.Secret) < minJWTSecretLen {
		return fmt.Errorf("C5_JWT_SECRET too short: need >= %d bytes (got %d)", minJWTSecretLen, len(c.JWT.Secret))
	}
	// Trusted-proxy entries feed gin.SetTrustedProxies; a malformed CIDR/IP would
	// silently break client-IP resolution (a rate-limiter bypass). Fail fast.
	for _, p := range c.Server.TrustedProxies {
		if _, err := netip.ParsePrefix(p); err == nil {
			continue
		}
		if _, err := netip.ParseAddr(p); err == nil {
			continue
		}
		return fmt.Errorf("C5_SERVER_TRUSTED_PROXIES: invalid CIDR/IP %q", p)
	}
	// CORS allow-list: required in prod (the SPA is served cross-origin from EdgeOne
	// Pages). Each entry must be an exact https Origin — no wildcard, no path — so the
	// middleware can echo it safely and never reflect an attacker-controlled value.
	if c.Env == "prod" {
		if len(c.Server.AllowedOrigins) == 0 {
			return fmt.Errorf("C5_CORS_ALLOWED_ORIGINS is required in prod (exact https origins, comma-separated)")
		}
		for _, o := range c.Server.AllowedOrigins {
			if err := validateOrigin(o); err != nil {
				return fmt.Errorf("C5_CORS_ALLOWED_ORIGINS: %w", err)
			}
		}
	}
	return nil
}

// validateOrigin enforces that o is an exact https web Origin: a scheme+host with
// no wildcard, path, query or fragment. Anything looser (e.g. "*", a path, or
// http://) would let the CORS middleware reflect an unsafe Access-Control-Allow-Origin.
func validateOrigin(o string) error {
	if o == "" || strings.Contains(o, "*") {
		return fmt.Errorf("invalid origin %q (no wildcard/empty)", o)
	}
	u, err := url.Parse(o)
	if err != nil || u.Scheme != "https" || u.Host == "" ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid origin %q (want https://host, no path/query/fragment)", o)
	}
	return nil
}

// minJWTSecretLen is the minimum acceptable HS256 secret length in bytes.
const minJWTSecretLen = 32

// splitCSV splits a comma-separated env value into a trimmed, empty-free slice,
// returning nil for blank input. viper's GetStringSlice does not reliably split a
// single comma-joined env var, so trusted-proxies are parsed explicitly.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
