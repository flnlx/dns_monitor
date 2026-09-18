//go:build windows

package winservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const Name = "DNSMonitor"

type Status struct {
	Installed bool   `json:"installed"`
	State     string `json:"state"`
	CanManage bool   `json:"can_manage"`
	Message   string `json:"message,omitempty"`
}

func IsService() bool { ok, _ := svc.IsWindowsService(); return ok }

func Inspect() Status {
	status := Status{State: "not_installed", CanManage: true}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		status.State = "unavailable"
		status.Message = err.Error()
		return status
	}
	defer windows.CloseServiceHandle(manager)
	name, _ := windows.UTF16PtrFromString(Name)
	handle, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			status.State = "unavailable"
			status.Message = err.Error()
		}
		return status
	}
	s := &mgr.Service{Name: Name, Handle: handle}
	defer s.Close()
	status.Installed = true
	q, err := s.Query()
	if err != nil {
		status.State = "unknown"
		status.Message = err.Error()
		return status
	}
	switch q.State {
	case svc.Running:
		status.State = "running"
	case svc.Stopped:
		status.State = "stopped"
	case svc.StartPending:
		status.State = "starting"
	case svc.StopPending:
		status.State = "stopping"
	default:
		status.State = "pending"
	}
	return status
}
func waitStopped(s *mgr.Service) error {
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		q, e := s.Query()
		if e != nil {
			return e
		}
		if q.State == svc.Stopped {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("服务停止超时，请在 Windows 服务管理器检查")
}

// Manage is called only by the explicit administrator CLI/helper action.
func Manage(action, exe, dataDir, doggoPath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("请以管理员身份运行: %w", err)
	}
	defer m.Disconnect()
	if action == "install" {
		existing, e := m.OpenService(Name)
		if e == nil {
			existing.Close()
			return fmt.Errorf("服务已经安装；移动目录后请卸载再安装")
		}
		if !errors.Is(e, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return e
		}
		s, e := m.CreateService(Name, exe, mgr.Config{DisplayName: "DNS Monitor 本地 DNS 监测", Description: "Portable DNS monitoring with local web dashboard", StartType: mgr.StartAutomatic, DelayedAutoStart: true}, "--data-dir", dataDir, "--doggo", doggoPath, "service-run")
		if e != nil {
			return e
		}
		return s.Close()
	}
	s, err := m.OpenService(Name)
	if err != nil {
		return err
	}
	defer s.Close()
	switch action {
	case "start":
		return s.Start()
	case "stop", "restart", "uninstall":
		q, e := s.Query()
		if e != nil {
			return e
		}
		if q.State != svc.Stopped {
			if _, e = s.Control(svc.Stop); e != nil && !errors.Is(e, windows.ERROR_SERVICE_NOT_ACTIVE) {
				return e
			}
			if e = waitStopped(s); e != nil {
				return e
			}
		}
		if action == "restart" {
			return s.Start()
		}
		if action == "uninstall" {
			return s.Delete()
		}
		return nil
	default:
		return fmt.Errorf("未知服务操作 %q", action)
	}
}

// Request starts a separate helper so stopping/restarting this service can finish
// after the HTTP response. Desktop requests use the native Windows UAC prompt.
func Request(action, exe, dataDir, doggoPath string) error {
	args := []string{"--data-dir", dataDir, "--doggo", doggoPath, "--helper-delay", "1s", "service", action}
	if windows.GetCurrentProcessToken().IsElevated() {
		cmd := exec.Command(exe, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	}
	params := ""
	for _, arg := range args {
		if params != "" {
			params += " "
		}
		params += syscall.EscapeArg(arg)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	param, _ := windows.UTF16PtrFromString(params)
	return windows.ShellExecute(0, verb, file, param, nil, windows.SW_HIDE)
}

type handler struct{ run func(context.Context) error }

func (h handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status <- svc.Status{State: svc.StartPending}
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	status <- current
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case r := <-requests:
			switch r.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
					return false, 0
				case <-time.After(20 * time.Second):
					return true, 2
				}
			}
		}
	}
}
func Run(run func(context.Context) error) error { return svc.Run(Name, handler{run: run}) }
func OpenBrowser(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, _ := windows.UTF16PtrFromString(url)
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}
func ProcessID() string { return strconv.Itoa(os.Getpid()) }
