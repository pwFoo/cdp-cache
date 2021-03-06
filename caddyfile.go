package httpcache

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// CacheType is the type of cache which means the backend for storing
// cache content.
type CacheType string

const (
	file     CacheType = "file"
	redis    CacheType = "redis"
	inMemory CacheType = "in_memory"
)

// BYTE represents the num of byte
// 1MB = 2^10 BYTE
// 1GB = 2^10 MB
const (
	BYTE = 1 << (iota * 10)
	KB
	MB
	GB
)

var (
	defaultStatusHeader       = "X-Cache-Status"
	defaultLockTimeout        = time.Duration(5) * time.Minute
	defaultMaxAge             = time.Duration(5) * time.Minute
	defaultPath               = ""
	defaultCacheType          = file
	defaultcacheBucketsNum    = 256
	defaultCacheMaxMemorySize = GB // default is 1 GB
	defaultCacheKeyTemplate   = "{http.request.method} {http.request.host}{http.request.uri.path}?{http.request.uri.query}"
	// the key is refereced from github.com/caddyserver/caddy/v2/modules/caddyhttp.addHTTPVarsToReplacer
)

const (
	keyStatusHeader       = "status_header"
	keyLockTimeout        = "lock_timeout"
	keyDefaultMaxAge      = "default_max_age"
	keyPath               = "path"
	keyMatchHeader        = "match_header"
	keyMatchPath          = "match_path"
	keyCacheKey           = "cache_key"
	keyCacheBucketsNum    = "cache_bucket_num"
	keyCacheMaxMemorySzie = "cache_max_memory_size"
	keyCacheType          = "cache_type"
)

func init() {
	httpcaddyfile.RegisterHandlerDirective("http_cache", parseCaddyfile)
}

// Config is the configuration for cache process
type Config struct {
	Type               CacheType                `json:"type,omitempty"`
	StatusHeader       string                   `json:"status_header,omitempty"`
	DefaultMaxAge      time.Duration            `json:"default_max_age,omitempty"`
	LockTimeout        time.Duration            `json:"lock_timeout,omitempty"`
	RuleMatchersRaws   []RuleMatcherRawWithType `json:"rule_matcher_raws,omitempty"`
	RuleMatchers       []RuleMatcher            `json:"-"`
	CacheBucketsNum    int                      `json:"cache_buckets_num,omitempty"`
	CacheMaxMemorySize int                      `json:"cache_max_memory_size,omitempty"`
	Path               string                   `json:"path,omitempty"`
	CacheKeyTemplate   string                   `json:"cache_key_template,omitempty"`
}

func getDefaultConfig() *Config {
	return &Config{
		StatusHeader:       defaultStatusHeader,
		DefaultMaxAge:      defaultMaxAge,
		LockTimeout:        defaultLockTimeout,
		RuleMatchersRaws:   []RuleMatcherRawWithType{},
		RuleMatchers:       []RuleMatcher{},
		CacheBucketsNum:    defaultcacheBucketsNum,
		CacheMaxMemorySize: defaultCacheMaxMemorySize,
		Path:               defaultPath,
		Type:               defaultCacheType,
		CacheKeyTemplate:   defaultCacheKeyTemplate,
	}
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	// call Handler UnmarshalCaddyfile
	hr := new(Handler)
	err := hr.UnmarshalCaddyfile(h.Dispenser)
	if err != nil {
		return nil, err
	}

	return hr, nil
}

// UnmarshalCaddyfile sets up the handler from Caddyfile
//
// :4000 {
//     reverse_proxy yourserver:5000
//     http_cache {
//         match_path /assets
//         match_header Content-Type image/jpg image/png
//         status_header X-Cache-Status
//         default_max_age 15m
//         path /tmp/caddy-cache
//     }
// }
func (h *Handler) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	config := getDefaultConfig()

	for d.Next() {

		for d.NextBlock(0) {
			parameter := d.Val()
			args := d.RemainingArgs()

			switch parameter {
			case keyStatusHeader:
				if len(args) != 1 {
					return d.Err("Invalid usage of status_header in cache config.")
				}
				config.StatusHeader = args[0]

			case keyCacheType:
				if len(args) != 1 {
					return d.Err("Invalid usage of cache_type in cache config.")
				}
				config.Type = CacheType(args[0])

			case keyLockTimeout:
				if len(args) != 1 {
					return d.Err("Invalid usage of lock_timeout in cache config")
				}
				duration, err := time.ParseDuration(parameter)
				if err != nil {
					return d.Err(fmt.Sprintf("%s:%s , %s", keyLockTimeout, "invalid duration", parameter))
				}
				config.LockTimeout = duration

			case keyDefaultMaxAge:
				if len(args) != 1 {
					return d.Err("Invalid usage of default_max_age in cache config.")
				}

				duration, err := time.ParseDuration(parameter)
				if err != nil {
					return d.Err(fmt.Sprintf("%s:%s, %s", keyDefaultMaxAge, "Invalid duration ", parameter))
				}
				config.DefaultMaxAge = duration

			case keyPath:
				if len(args) != 1 {
					return d.Err("Invalid usage of path in cache config.")
				}
				config.Path = args[0]

			case keyMatchHeader:
				if len(args) < 2 {
					return d.Err("Invalid usage of match_header in cache config.")
				}

				cacheRule := &HeaderRuleMatcher{Header: args[0], Value: args[1:]}
				d, _ := json.Marshal(cacheRule)

				config.RuleMatchersRaws = append(config.RuleMatchersRaws, RuleMatcherRawWithType{
					Type: MatcherTypePath,
					Data: d,
				})
				// config.RuleMatchers = append(config.RuleMatchers, cacheRule)

			case keyMatchPath:
				if len(args) != 1 {
					return d.Err("Invalid usage of match_path in cache config.")
				}
				cacheRule := &PathRuleMatcher{Path: args[0]}
				d, _ := json.Marshal(cacheRule)

				config.RuleMatchersRaws = append(config.RuleMatchersRaws, RuleMatcherRawWithType{
					Type: MatcherTypePath,
					Data: d,
				})
				// config.RuleMatchers = append(config.RuleMatchers, cacheRule)

			case keyCacheKey:
				if len(args) != 1 {
					return d.Err(fmt.Sprintf("Invalid usage of %s in cache config.", keyCacheKey))
				}
				config.CacheKeyTemplate = args[0]

			case keyCacheBucketsNum:
				if len(args) != 1 {
					return d.Err(fmt.Sprintf("Invalid usage of %s in cache config.", keyCacheBucketsNum))
				}
				num, err := strconv.Atoi(args[0])
				if err != nil {
					return d.Err(fmt.Sprintf("Invalid usage of %s, %s", keyCacheBucketsNum, err.Error()))
				}
				config.CacheBucketsNum = num

			default:
				return d.Err("Unknown cache parameter: " + parameter)
			}
		}
	}

	h.Config = config

	return nil
}

// Interface guards
var (
	_ caddyfile.Unmarshaler = (*Handler)(nil)
)
