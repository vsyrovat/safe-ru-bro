package xauth

import (
	"fmt"
	"os"
	"os/exec"
)

// Write builds an X authority file that matches any hostname, so the container
// can open windows on the host display. cleanup removes the file.
func Write(display string) (path string, cleanup func(), err error) {
	file, err := os.CreateTemp("", "safe-ru-bro-xauth-")
	if err != nil {
		return "", nil, err
	}
	path = file.Name()
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		os.Remove(path)
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", nil, err
	}
	cleanup = func() { os.Remove(path) }

	if err := mergeFamilyWild(display, path); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func mergeFamilyWild(display, dest string) error {
	nlist := exec.Command("xauth", "nlist", display)
	rewrite := exec.Command("sed", "-e", "s/^..../ffff/")
	merge := exec.Command("xauth", "-f", dest, "nmerge", "-")

	nlistOut, err := nlist.StdoutPipe()
	if err != nil {
		return err
	}
	rewrite.Stdin = nlistOut
	rewriteOut, err := rewrite.StdoutPipe()
	if err != nil {
		return err
	}
	merge.Stdin = rewriteOut

	nlistErr, rewriteErr, mergeErr := &stringsBuilder{}, &stringsBuilder{}, &stringsBuilder{}
	nlist.Stderr = nlistErr
	rewrite.Stderr = rewriteErr
	merge.Stderr = mergeErr

	if err := merge.Start(); err != nil {
		return err
	}
	if err := rewrite.Start(); err != nil {
		return err
	}
	if err := nlist.Start(); err != nil {
		return err
	}
	if err := nlist.Wait(); err != nil {
		rewrite.Wait()
		merge.Wait()
		return fmt.Errorf("xauth nlist: %w%s", err, nlistErr.suffix())
	}
	if err := rewrite.Wait(); err != nil {
		merge.Wait()
		return fmt.Errorf("xauth rewrite: %w%s", err, rewriteErr.suffix())
	}
	if err := merge.Wait(); err != nil {
		return fmt.Errorf("xauth nmerge: %w%s", err, mergeErr.suffix())
	}
	return nil
}

type stringsBuilder struct {
	b []byte
}

func (s *stringsBuilder) Write(p []byte) (int, error) {
	s.b = append(s.b, p...)
	return len(p), nil
}

func (s *stringsBuilder) suffix() string {
	if len(s.b) == 0 {
		return ""
	}
	return ": " + string(s.b)
}
