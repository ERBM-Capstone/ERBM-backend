// Package migration discovers real migrations without treating SQL templates as versions.
package migration

import (
	"path/filepath"
	"regexp"
	"strings"
)

var name = regexp.MustCompile(`^[0-9]+_[a-z0-9_]+\.up\.sql$`)

func Versions(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(files))
	for _, file := range files {
		base := filepath.Base(file)
		if name.MatchString(base) {
			versions = append(versions, strings.TrimSuffix(base, ".up.sql"))
		}
	}
	return versions, nil
}
