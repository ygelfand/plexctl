package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestSaveKeepsCredentialsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits")
	}
	viper.Reset()
	t.Cleanup(viper.Reset)
	for _, name := range []string{"new", "previously public"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if name == "previously public" {
				if err := os.WriteFile(path, []byte("token: old\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := Config{ConfigPath: path, Token: "private-account"}
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0600 {
				t.Fatalf("credential file permissions = %o, want 600", info.Mode().Perm())
			}
			loaded := viper.New()
			loaded.SetConfigFile(path)
			if err := loaded.ReadInConfig(); err != nil || loaded.GetString("token") != cfg.Token {
				t.Fatalf("saved credentials did not reload: %v", err)
			}
		})
	}
}

func TestSaveFailurePreservesConfiguration(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	path := filepath.Join(t.TempDir(), "config.invalid")
	const original = "existing configuration"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{ConfigPath: path, Token: "replacement"}
	if err := cfg.Save(); err == nil {
		t.Fatal("unsupported configuration format was accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != original {
		t.Fatalf("failed save damaged the previous file: %v", err)
	}
}

func TestSavePreservesCredentialsAcrossFormats(t *testing.T) {
	for _, format := range []string{"yaml", "json", "toml"} {
		t.Run(format, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			cfg := Config{
				ConfigPath: filepath.Join(t.TempDir(), "config."+format), Token: "account-token",
				HomeUser:      HomeUser{AuthToken: "profile-token", AccessToken: "profile-resource", ServerID: "shared"},
				DefaultServer: "shared",
				Servers: map[string]Server{"shared": {
					Name: "Shared library", URL: "https://plex.invalid", Shared: true,
					AccessToken: "resource+&token", TokenSource: "source-fingerprint",
					Libraries: LibraryConfig{
						Order: []string{"1"},
						Settings: map[string]LibraryOptions{"1": {
							IconEmoji: "film", IconNF: "nf", IconASCII: "M", Hidden: true, ViewMode: ViewModeList,
						}},
					},
				}},
			}
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			loaded := viper.New()
			loaded.SetConfigFile(cfg.ConfigPath)
			if err := loaded.ReadInConfig(); err != nil {
				t.Fatal(err)
			}
			var got Config
			if err := loaded.Unmarshal(&got); err != nil {
				t.Fatal(err)
			}
			if got.Token != cfg.Token || got.HomeUser != cfg.HomeUser || got.DefaultServer != cfg.DefaultServer ||
				!reflect.DeepEqual(got.Servers, cfg.Servers) {
				t.Fatal("saved configuration lost credentials or nested library settings")
			}
			output, err := json.Marshal(cfg.Servers)
			if err != nil {
				t.Fatal(err)
			}
			encodedToken, err := json.Marshal(cfg.Servers["shared"].AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(output), string(encodedToken)) || strings.Contains(string(output), "source-fingerprint") {
				t.Fatal("runtime JSON output no longer redacts server credentials")
			}
		})
	}
}
