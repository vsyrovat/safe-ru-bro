package host

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// Session is the host identity and the folders kept beside the executable.
type Session struct {
	UID         string
	GID         string
	Groups      string
	Home        string
	Display     string
	Root        string
	Cache       string
	Data        string
	Downloads   string
	PulseSocket string
	PulseCookie string
	DRI         bool
}

// Detect creates cache, data, and downloads next to the executable and reads
// the host session those mounts need.
func Detect() (Session, error) {
	exe, err := os.Executable()
	if err != nil {
		return Session{}, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return Session{}, err
	}
	root := filepath.Dir(exe)

	session := Session{
		Root:      root,
		Cache:     filepath.Join(root, "cache"),
		Data:      filepath.Join(root, "data"),
		Downloads: filepath.Join(root, "downloads"),
	}
	for _, dir := range []string{session.Cache, session.Data, session.Downloads} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Session{}, err
		}
	}

	account, err := user.Current()
	if err != nil {
		return Session{}, err
	}
	session.UID = account.Uid
	session.GID = account.Gid
	session.Home = account.HomeDir

	groups, err := groupList(session.GID)
	if err != nil {
		return Session{}, err
	}
	session.Groups = groups

	session.Display = os.Getenv("DISPLAY")
	if session.Display == "" {
		return Session{}, fmt.Errorf("DISPLAY is not set")
	}

	socket := filepath.Join("/run/user", session.UID, "pulse", "native")
	if fileExists(socket) {
		session.PulseSocket = socket
	}
	cookie := filepath.Join(session.Home, ".config", "pulse", "cookie")
	if fileExists(cookie) {
		session.PulseCookie = cookie
	}
	if info, err := os.Stat("/dev/dri"); err == nil && info.IsDir() {
		session.DRI = true
	}
	return session, nil
}

func groupList(primary string) (string, error) {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return "", err
	}
	defer file.Close()

	var extra []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "Groups:") {
			continue
		}
		extra = strings.Fields(strings.TrimPrefix(line, "Groups:"))
		break
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	groups := []string{primary}
	seen := map[string]bool{primary: true}
	for _, group := range extra {
		if seen[group] {
			continue
		}
		seen[group] = true
		groups = append(groups, group)
	}
	return strings.Join(groups, ","), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
