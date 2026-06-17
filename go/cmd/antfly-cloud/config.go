package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	defaultAPIURL       = "https://platform.antfly.io/api/v1"
	defaultOIDCClientID = "cloudaf-cli"
	configDirName       = ".antfly/cloud"
	defaultConfigFile   = "config.yaml"
	authTypeOIDC        = "oidc"
	authTypePAT         = "pat"
)

type AuthConfig struct {
	Type         string    `json:"type,omitempty" yaml:"type,omitempty" mapstructure:"type"`
	Issuer       string    `json:"issuer,omitempty" yaml:"issuer,omitempty" mapstructure:"issuer"`
	ClientID     string    `json:"client_id,omitempty" yaml:"client_id,omitempty" mapstructure:"client_id"`
	IDToken      string    `json:"id_token,omitempty" yaml:"id_token,omitempty" mapstructure:"id_token"`
	RefreshToken string    `json:"refresh_token,omitempty" yaml:"refresh_token,omitempty" mapstructure:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at,omitempty" yaml:"expires_at,omitempty" mapstructure:"expires_at"`
	Token        string    `json:"token,omitempty" yaml:"token,omitempty" mapstructure:"token"`
}

type Config struct {
	APIURL   string     `json:"api_url" yaml:"api_url" mapstructure:"api_url"`
	Token    string     `json:"token,omitempty" yaml:"token,omitempty" mapstructure:"token"`
	Org      string     `json:"org,omitempty" yaml:"org,omitempty" mapstructure:"org"`
	Instance string     `json:"instance,omitempty" yaml:"instance,omitempty" mapstructure:"instance"`
	Auth     AuthConfig `json:"auth,omitempty" yaml:"auth,omitempty" mapstructure:"auth"`
}

type configFile struct {
	APIURL   string      `json:"api_url" yaml:"api_url" mapstructure:"api_url"`
	Token    string      `json:"token,omitempty" yaml:"token,omitempty" mapstructure:"token"`
	Org      string      `json:"org,omitempty" yaml:"org,omitempty" mapstructure:"org"`
	Instance string      `json:"instance,omitempty" yaml:"instance,omitempty" mapstructure:"instance"`
	Auth     *AuthConfig `json:"auth,omitempty" yaml:"auth,omitempty" mapstructure:"auth"`
}

func defaultConfig() Config { return Config{APIURL: defaultAPIURL} }

func (c Config) bearerToken() string {
	if c.Token != "" {
		return c.Token
	}
	switch c.Auth.Type {
	case authTypeOIDC:
		return c.Auth.IDToken
	case authTypePAT:
		return c.Auth.Token
	default:
		return ""
	}
}

func isCloudAFManagementToken(token string) bool {
	return strings.HasPrefix(strings.TrimSpace(token), "antfly_cloud_")
}

func configPath(override string) (string, error) {
	return configPathForWrite(override)
}

func loadConfig(pathOverride string) (Config, error) {
	cfg := defaultConfig()
	v, used, err := loadConfigViper(pathOverride)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if used == "" {
		return cfg, nil
	}
	if err := v.Unmarshal(&cfg, viper.DecodeHook(mapstructure.StringToTimeHookFunc(time.RFC3339))); err != nil {
		return cfg, fmt.Errorf("read %s: %w", used, err)
	}
	if cfg.APIURL == "" {
		cfg.APIURL = defaultAPIURL
	}
	return cfg, nil
}

func loadConfigViper(pathOverride string) (*viper.Viper, string, error) {
	candidates, err := configReadCandidates(pathOverride)
	if err != nil {
		return nil, "", err
	}
	for _, path := range candidates {
		v := viper.New()
		v.SetConfigFile(path)
		if filepath.Ext(path) == "" {
			v.SetConfigType("yaml")
		}
		v.SetEnvPrefix("antfly_cloud")
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if errors.As(err, &notFound) || errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, "", err
		}
		return v, v.ConfigFileUsed(), nil
	}
	return viper.New(), "", os.ErrNotExist
}

func configReadCandidates(pathOverride string) ([]string, error) {
	if pathOverride != "" {
		return explicitConfigCandidates(pathOverride), nil
	}
	if env := os.Getenv("ANTFLY_CLOUD_CONFIG"); env != "" {
		return explicitConfigCandidates(env), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	candidates := []string{}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, configFilesInDir(filepath.Join(cwd, configDirName))...)
	}
	candidates = append(candidates, configFilesInDir(filepath.Join(home, configDirName))...)
	return candidates, nil
}

func explicitConfigCandidates(path string) []string {
	if isConfigDirPath(path) {
		return configFilesInDir(path)
	}
	return []string{path}
}

func configFilesInDir(dir string) []string {
	return []string{
		filepath.Join(dir, "config.yaml"),
		filepath.Join(dir, "config.json"),
	}
}

func configPathForWrite(pathOverride string) (string, error) {
	if pathOverride != "" {
		return explicitConfigPathForWrite(pathOverride), nil
	}
	if env := os.Getenv("ANTFLY_CLOUD_CONFIG"); env != "" {
		return explicitConfigPathForWrite(env), nil
	}
	candidates, err := configReadCandidates("")
	if err != nil {
		return "", err
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, configDirName, defaultConfigFile), nil
}

func explicitConfigPathForWrite(path string) string {
	if isConfigDirPath(path) {
		return filepath.Join(path, defaultConfigFile)
	}
	return path
}

func isConfigDirPath(path string) bool {
	if info, err := os.Stat(path); err == nil {
		return info.IsDir()
	}
	base := filepath.Base(filepath.Clean(path))
	return base == configDirName || filepath.Ext(base) == ""
}

func saveConfig(pathOverride string, cfg Config) error {
	path, err := configPath(pathOverride)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	var b []byte
	var marshalErr error
	onDisk := configForDisk(cfg)
	if strings.EqualFold(filepath.Ext(path), ".json") {
		b, marshalErr = json.MarshalIndent(onDisk, "", "  ")
		if marshalErr == nil {
			b = append(b, '\n')
		}
	} else {
		b, marshalErr = yaml.Marshal(onDisk)
	}
	if marshalErr != nil {
		return marshalErr
	}
	return os.WriteFile(path, b, 0o600)
}

func configForDisk(cfg Config) configFile {
	out := configFile{
		APIURL:   cfg.APIURL,
		Token:    cfg.Token,
		Org:      cfg.Org,
		Instance: cfg.Instance,
	}
	if !isEmptyAuthConfig(cfg.Auth) {
		out.Auth = &cfg.Auth
	}
	return out
}

func isEmptyAuthConfig(cfg AuthConfig) bool {
	return cfg.Type == "" &&
		cfg.Issuer == "" &&
		cfg.ClientID == "" &&
		cfg.IDToken == "" &&
		cfg.RefreshToken == "" &&
		cfg.ExpiresAt.IsZero() &&
		cfg.Token == ""
}

func removeConfigToken(pathOverride string) error {
	cfg, err := loadConfig(pathOverride)
	if err != nil {
		return err
	}
	cfg.Token = ""
	cfg.Auth = AuthConfig{}
	return saveConfig(pathOverride, cfg)
}

func applyEnv(cfg Config) Config {
	if v := os.Getenv("ANTFLY_CLOUD_API_URL"); v != "" {
		cfg.APIURL = v
	}
	if v := os.Getenv("ANTFLY_CLOUD_TOKEN"); v != "" {
		cfg.Token = v
		cfg.Auth = AuthConfig{}
	}
	if v := os.Getenv("ANTFLY_CLOUD_ORG"); v != "" {
		cfg.Org = v
	}
	if v := os.Getenv("ANTFLY_CLOUD_INSTANCE"); v != "" {
		cfg.Instance = v
	}
	return cfg
}
