package brand

import (
	"fmt"
	"os"
	"path/filepath"
)

// Locales rewrites Ungoogled Chromium locale packs so window titles end with
// SafeRuBro. dir defaults to the portable build's locale directory.
func Locales(dir string) error {
	if dir == "" {
		dir = "/opt/ungoogled-chromium/locales"
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.pak"))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no locale packs in %s", dir)
	}

	replaced := 0
	branded := 0
	for _, path := range paths {
		n, already, err := rewriteFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		replaced += n
		if already {
			branded++
		}
	}
	if replaced == 0 && branded == 0 {
		return fmt.Errorf("chromium window title suffix was not found in %s", dir)
	}
	return nil
}

func rewriteFile(path string) (int, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false, err
	}
	out, n, err := rewrite(data)
	if err != nil {
		return 0, false, err
	}
	if n == 0 {
		return 0, containsNewSuffix(data), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return 0, false, err
	}
	tmp := path + ".brand"
	if err := os.WriteFile(tmp, out, info.Mode()); err != nil {
		return 0, false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return 0, false, err
	}
	return n, true, nil
}
