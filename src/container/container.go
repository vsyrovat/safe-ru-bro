package container

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"safe-ru-bro/src/config"
	"safe-ru-bro/src/host"
)

const imageName = "safe-ru-bro"

// Runner builds the embedded image and starts Ungoogled Chromium in a container.
type Runner struct {
	docker string
	files  fs.FS
}

func New(files fs.FS) (Runner, error) {
	docker, err := lookupDocker()
	if err != nil {
		return Runner{}, err
	}
	return Runner{docker: docker, files: files}, nil
}

func (r Runner) Build() error {
	dir, err := os.MkdirTemp("", "safe-ru-bro-image-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	if err := fs.WalkDir(r.files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := fs.ReadFile(r.files, name)
		if err != nil {
			return err
		}
		dest := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, body, 0o644)
	}); err != nil {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "safe-ru-bro"), binary, 0o755); err != nil {
		return err
	}

	cmd := exec.Command(r.docker, "build", "-t", imageName, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	return nil
}

func (r Runner) Run(session host.Session, xauthority string, upstream *config.Proxy, flags, urls []string) error {
	args := []string{
		"run", "--rm", "--name", imageName,
		"--shm-size=1g",
		"--security-opt", "seccomp=unconfined",
		"--security-opt", "no-new-privileges",
		"-e", "DISPLAY=" + session.Display,
		"-e", "XAUTHORITY=/tmp/.Xauthority",
		"-e", "HOME=/home/browser",
		"-e", "HOST_UID=" + session.UID,
		"-e", "HOST_GID=" + session.GID,
		"-e", "HOST_GROUPS=" + session.Groups,
		"-v", "/tmp/.X11-unix:/tmp/.X11-unix",
		"-v", xauthority + ":/tmp/.Xauthority:ro",
		"-v", session.Downloads + ":/home/browser/Downloads",
		"-v", session.Data + ":/home/browser/data",
		"-v", session.Cache + ":/home/browser/cache",
	}
	if session.PulseSocket != "" {
		args = append(args,
			"-e", "PULSE_SERVER=unix:/tmp/pulse/native",
			"-v", session.PulseSocket+":/tmp/pulse/native",
		)
	}
	if session.PulseCookie != "" {
		args = append(args,
			"-e", "PULSE_COOKIE=/tmp/pulse-cookie",
			"-v", session.PulseCookie+":/tmp/pulse-cookie:ro",
		)
	}
	if session.DRI {
		args = append(args, "--device", "/dev/dri")
	}
	if upstream != nil {
		args = append(args,
			"--cap-add", "NET_ADMIN",
			"--device", "/dev/net/tun",
			"--sysctl", "net.ipv6.conf.all.disable_ipv6=1",
			"--sysctl", "net.ipv6.conf.default.disable_ipv6=1",
			"-e", "PROXY_HOST="+upstream.Host,
			"-e", "PROXY_PORT="+upstream.Port,
			"-e", "PROXY_USER="+upstream.User,
			"-e", "PROXY_PASS="+upstream.Pass,
		)
	}
	args = append(args, imageName)
	args = append(args, flags...)
	args = append(args, urls...)

	cmd := exec.Command(r.docker, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Own process group so Ctrl+C reaches this process, which can stop the
	// container and let Ungoogled Chromium write its tabs before exiting.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker run: %w", err)
	}

	var stopping atomic.Bool
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		stopping.Store(true)
		stop := exec.Command(r.docker, "stop", "-t", "20", imageName)
		stop.Stdout = os.Stdout
		stop.Stderr = os.Stderr
		_ = stop.Run()
	}()

	if err := cmd.Wait(); err != nil && !stopping.Load() {
		return fmt.Errorf("docker run: %w", err)
	}
	return nil
}

func lookupDocker() (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, "docker")
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("docker not found in PATH")
}
