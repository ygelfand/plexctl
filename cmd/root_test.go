package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestReadConfigFileReportsErrors(t *testing.T) {
	for _, tt := range []struct {
		name     string
		contents string
		create   bool
		explicit bool
		wantErr  bool
	}{
		{name: "optional missing"},
		{name: "explicit missing", explicit: true, wantErr: true},
		{name: "optional malformed", contents: "servers: [", create: true, wantErr: true},
		{name: "explicit malformed", contents: "servers: [", create: true, explicit: true, wantErr: true},
		{name: "valid", contents: "default_to_tui: false\n", create: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".plexctl.yaml")
			if tt.create {
				if err := os.WriteFile(path, []byte(tt.contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			v := viper.New()
			v.SetConfigType("yaml")
			if tt.explicit {
				v.SetConfigFile(path)
			} else {
				v.AddConfigPath(dir)
				v.SetConfigName(".plexctl")
			}
			gotPath, err := readConfigFile(v, tt.explicit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("readConfigFile error = %v; want error %v", err, tt.wantErr)
			}
			if tt.create && !tt.wantErr && gotPath != path {
				t.Fatalf("config path = %q, want %q", gotPath, path)
			}
		})
	}
}
