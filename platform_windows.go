//go:build windows

package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"mmonitor/internal/config"
	"mmonitor/internal/icon"
)

//go:embed assets/logo.svg
var logoSVGBytes []byte

var osExecutable = os.Executable

const (
	windowsDataDirName  = "data"
	windowsDatabaseName = "mmonitor.db"
	windowsLogName      = "mmonitor.log"

	singletonMutexName  = `Local\MMonitor.Singleton`
	autostartRunKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	autostartValueName  = "MMonitor"
	runtimeMetadataDir  = "MMonitor"
)

var (
	secondInstanceWait = 5 * time.Second
	trayReadyTimeout   = 5 * time.Second
)

func windowsDataDir() (string, error) {
	exePath, err := resolveExecutablePath()
	if err != nil {
		return "", err
	}
	exeDir := filepath.Dir(exePath)
	if strings.TrimSpace(exeDir) == "" {
		return "", errors.New("executable path has no directory")
	}
	return filepath.Join(exeDir, windowsDataDirName), nil
}

func resolveExecutablePath() (string, error) {
	exePath, err := osExecutable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	abs, err := filepath.Abs(exePath)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	return abs, nil
}

func windowsDatabasePath() (string, error) {
	dataDir, err := windowsDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, windowsDatabaseName), nil
}

func dashboardURLFromAddr(addr net.Addr) (string, error) {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "", fmt.Errorf("parse listen address %q: %w", addr.String(), err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber <= 0 || portNumber > 65535 {
		return "", fmt.Errorf("listen address %q has no usable port", addr.String())
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	dashboard := url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), Path: "/"}
	return dashboard.String(), nil
}

type runtimeMetadata struct {
	PID       int    `json:"pid"`
	SessionID uint32 `json:"sessionId"`
	URL       string `json:"url"`
	StartedAt int64  `json:"startedAt"`
}

func runtimeMetadataPath(sessionID uint32) (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", fmt.Errorf("resolve LocalAppData: %w", err)
	}
	return filepath.Join(base, runtimeMetadataDir, fmt.Sprintf("runtime-%d.json", sessionID)), nil
}

func publishRuntimeMetadata(path string, meta runtimeMetadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create runtime metadata directory: %w", err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode runtime metadata: %w", err)
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return fmt.Errorf("write runtime metadata: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("publish runtime metadata: %w", err)
	}
	return nil
}

func readRuntimeMetadata(path string) (runtimeMetadata, error) {
	var meta runtimeMetadata
	data, err := os.ReadFile(path)
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, fmt.Errorf("decode runtime metadata %s: %w", path, err)
	}
	return meta, nil
}

func validateMetadataURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse dashboard url: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("unacceptable dashboard url %q", raw)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("dashboard url %q has no host", raw)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("dashboard url %q has no usable port", raw)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("dashboard url %q has an unexpected path", raw)
	}
	return nil
}

func deleteRuntimeMetadataIfOwner(path string, pid int, sessionID uint32) {
	meta, err := readRuntimeMetadata(path)
	if err != nil || meta.PID != pid || meta.SessionID != sessionID {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("remove runtime metadata %s: %v", path, err)
	}
}

func autostartCommand(exePath string) string {
	return `"` + exePath + `"`
}

type autoStartStore interface {
	Enabled(exePath string) (bool, error)
	Enable(exePath string) error
	Disable() error
}

type registryAutoStartStore struct {
	runKeyPath string
}

func (s registryAutoStartStore) Enabled(exePath string) (bool, error) {
	value, err := s.readValue()
	if err != nil {
		return false, err
	}
	return value == autostartCommand(exePath), nil
}

func (s registryAutoStartStore) readValue() (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, s.runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(autostartValueName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func (s registryAutoStartStore) Enable(exePath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, s.runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(autostartValueName, autostartCommand(exePath))
}

func (s registryAutoStartStore) Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, s.runKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(autostartValueName); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func acquireSingletonMutex(name string) (windows.Handle, bool, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, false, fmt.Errorf("invalid mutex name %q: %w", name, err)
	}
	handle, err := windows.CreateMutex(nil, false, namePtr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return handle, true, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("create singleton mutex: %w", err)
	}
	return handle, false, nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil || handle == 0 {
		return false
	}
	_ = windows.CloseHandle(handle)
	return true
}

func shellOpenURL(target string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

func openDashboardFromMetadata(path string, within time.Duration, opener func(string) error) error {
	deadline := time.Now().Add(within)
	for {
		meta, err := readRuntimeMetadata(path)
		if err == nil {
			if validateErr := validateMetadataURL(meta.URL); validateErr != nil {
				log.Printf("ignoring invalid runtime metadata url: %v", validateErr)
			} else if processAlive(meta.PID) {
				return opener(meta.URL)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("read runtime metadata %s: %v", path, err)
		}
		if !time.Now().Before(deadline) {
			return errors.New("no running instance published a valid dashboard url")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

const (
	rotatingLogMaxSize      = 5 * 1024 * 1024
	rotatingLogBackupSuffix = ".1"
)

type rotatingLogWriter struct {
	mu         sync.Mutex
	path       string
	backupPath string
	file       *os.File
	size       int64
	errCh      chan error
	stickyErr  error
}

func newRotatingLogWriter(path string) (*rotatingLogWriter, error) {
	w := &rotatingLogWriter{
		path:       path,
		backupPath: path + rotatingLogBackupSuffix,
		errCh:      make(chan error, 1),
	}
	if err := w.openCurrentLocked(); err != nil {
		w.stickyErr = err
		return w, fmt.Errorf("open log file %s: %w", path, err)
	}
	return w, nil
}

func (w *rotatingLogWriter) Errors() <-chan error {
	return w.errCh
}

func (w *rotatingLogWriter) stickyError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stickyErr
}

func (w *rotatingLogWriter) notify(err error) {
	select {
	case w.errCh <- err:
	default:
	}
}

func (w *rotatingLogWriter) openCurrentLocked() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotatingLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.openCurrentLocked(); err != nil {
			w.stickyErr = err
			return 0, fmt.Errorf("reopen log file %s: %w", w.path, err)
		}
	}
	if w.size+int64(len(p)) > rotatingLogMaxSize {
		if err := w.rotateLocked(); err != nil {
			w.notify(fmt.Errorf("rotate log: %w", err))
			if w.file == nil {
				return 0, err
			}
		}
	}
	n, err := w.file.Write(p)
	if n > 0 {
		w.size += int64(n)
	}
	if err != nil {
		w.notify(fmt.Errorf("write log: %w", err))
		return n, err
	}
	return n, nil
}

func (w *rotatingLogWriter) rotateLocked() error {
	var errs []error
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close current log: %w", err))
		}
		w.file = nil
		w.size = 0
	}
	if err := os.Remove(w.backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("remove old log backup: %w", err))
	}
	if err := os.Rename(w.path, w.backupPath); err != nil {
		errs = append(errs, fmt.Errorf("rename log to backup: %w", err))
	}
	if err := w.openCurrentLocked(); err != nil {
		w.stickyErr = err
		errs = append(errs, fmt.Errorf("reopen current log: %w", err))
		return errors.Join(errs...)
	}
	return errors.Join(errs...)
}

func (w *rotatingLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

var windowsLog *rotatingLogWriter
var showMessageBox = showErrorMessageBox

type trayRunner interface {
	Run(onReady, onExit func())
	Quit()
}

type systrayRunner struct{}

func (systrayRunner) Run(onReady, onExit func()) { systray.Run(onReady, onExit) }
func (systrayRunner) Quit()                      { systray.Quit() }

type trayMenu struct {
	openPageClicks  <-chan struct{}
	autoStartClicks <-chan struct{}
	quitClicks      <-chan struct{}
	setAutoStart    func(bool)
}

type traySupervisor struct {
	app              *application
	runner           trayRunner
	store            autoStartStore
	exePath          string
	dashboardURL     string
	openURL          func(string) error
	logWriter        *rotatingLogWriter
	interruptCh      <-chan os.Signal
	menuCh           <-chan trayMenu
	initialAutoStart bool
	quitRequested    *atomic.Bool
	outcome          chan<- error
}

func sendOutcome(outcome chan<- error, err error) {
	select {
	case outcome <- err:
	default:
	}
}

func (s *traySupervisor) run(stop <-chan struct{}) {
	var menu trayMenu
	autoStartEnabled := s.initialAutoStart
	logFaultReported := false

	for {
		select {
		case m, ok := <-s.menuCh:
			if ok {
				menu = m
			}
		case err := <-s.app.errors():
			log.Printf("server error: %v", err)
			shutdownErr := s.app.shutdown()
			s.quitRequested.Store(true)
			sendOutcome(s.outcome, errors.Join(err, shutdownErr))
			s.runner.Quit()
			return
		case err := <-s.logWriter.Errors():
			log.Printf("log writer failure: %v", err)
			if !logFaultReported {
				logFaultReported = true
				showMessageBox("MMonitor 日志写入遇到磁盘故障，最近的日志可能丢失。")
			}
		case <-s.interruptCh:
			s.quitRequested.Store(true)
			sendOutcome(s.outcome, s.app.shutdown())
			s.runner.Quit()
			return
		case <-stop:
			return
		case <-menu.quitClicks:
			s.quitRequested.Store(true)
			sendOutcome(s.outcome, s.app.shutdown())
			s.runner.Quit()
			return
		case <-menu.openPageClicks:
			if err := s.openURL(s.dashboardURL); err != nil {
				log.Printf("open dashboard %s: %v", s.dashboardURL, err)
				showMessageBox(fmt.Sprintf("打开面板失败：\n\n%v", err))
			}
		case <-menu.autoStartClicks:
			if autoStartEnabled {
				if err := s.store.Disable(); err != nil {
					log.Printf("disable autostart: %v", err)
					showMessageBox(fmt.Sprintf("关闭开机自动启动失败：\n\n%v", err))
					continue
				}
				autoStartEnabled = false
				menu.setAutoStart(false)
			} else {
				if err := s.store.Enable(s.exePath); err != nil {
					log.Printf("enable autostart: %v", err)
					showMessageBox(fmt.Sprintf("设置开机自动启动失败：\n\n%v", err))
					continue
				}
				autoStartEnabled = true
				menu.setAutoStart(true)
			}
		}
	}
}

const wmQuit = 0x0012

var procPostThreadMessageW = windows.NewLazySystemDLL("user32.dll").NewProc("PostThreadMessageW")

func postThreadQuitMessage(threadID uint32) {
	_, _, _ = procPostThreadMessageW.Call(uintptr(threadID), wmQuit, 0, 0)
}

func runPlatform() error {
	cfg := config.Load()

	exePath, err := resolveExecutablePath()
	if err != nil {
		return err
	}

	var sessionID uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sessionID); err != nil {
		return fmt.Errorf("resolve session id: %w", err)
	}
	metaPath, err := runtimeMetadataPath(sessionID)
	if err != nil {
		return err
	}

	mutexHandle, alreadyRunning, err := acquireSingletonMutex(singletonMutexName)
	if err != nil {
		return err
	}
	if alreadyRunning {
		_ = windows.CloseHandle(mutexHandle)
		log.Printf("another mmonitor instance is running in this session; opening its dashboard")
		if openErr := openDashboardFromMetadata(metaPath, secondInstanceWait, shellOpenURL); openErr != nil {
			showErrorMessageBox(fmt.Sprintf("MMonitor 已经在运行，但打开面板失败：\n\n%v", openErr))
			return openErr
		}
		return nil
	}
	defer windows.CloseHandle(mutexHandle)

	dataDir, err := windowsDataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	logWriter, err := newRotatingLogWriter(filepath.Join(dataDir, windowsLogName))
	if err != nil {
		return err
	}
	windowsLog = logWriter
	log.SetOutput(logWriter)
	log.Printf("mmonitor v%s starting (pid %d, session %d)", config.AppVersion, os.Getpid(), sessionID)

	dbPath, err := windowsDatabasePath()
	if err != nil {
		return err
	}

	app, err := newApplication(cfg, dbPath)
	if err != nil {
		return err
	}

	dashboardURL, err := dashboardURLFromAddr(app.listener.Addr())
	if err != nil {
		_ = app.shutdown()
		return err
	}

	app.start()
	if err := logWriter.stickyError(); err != nil {
		_ = app.shutdown()
		return fmt.Errorf("log writer: %w", err)
	}

	meta := runtimeMetadata{
		PID:       os.Getpid(),
		SessionID: sessionID,
		URL:       dashboardURL,
		StartedAt: time.Now().UnixMilli(),
	}
	if err := publishRuntimeMetadata(metaPath, meta); err != nil {
		_ = app.shutdown()
		return fmt.Errorf("publish runtime metadata: %w", err)
	}
	log.Printf("dashboard url: %s", dashboardURL)

	store := registryAutoStartStore{runKeyPath: autostartRunKeyPath}
	initialAutoStart := false
	if enabled, err := store.Enabled(exePath); err != nil {
		log.Printf("read autostart state: %v", err)
	} else {
		initialAutoStart = enabled
	}

	readyCh := make(chan struct{})
	menuCh := make(chan trayMenu, 1)
	stopCh := make(chan struct{})
	outcomeCh := make(chan error, 1)
	var quitRequested atomic.Bool
	var readyTimedOut atomic.Bool

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	runner := systrayRunner{}
	supervisor := &traySupervisor{
		app:              app,
		runner:           runner,
		store:            store,
		exePath:          exePath,
		dashboardURL:     dashboardURL,
		openURL:          shellOpenURL,
		logWriter:        logWriter,
		interruptCh:      sigCh,
		menuCh:           menuCh,
		initialAutoStart: initialAutoStart,
		quitRequested:    &quitRequested,
		outcome:          outcomeCh,
	}
	go supervisor.run(stopCh)

	mainThreadID := windows.GetCurrentThreadId()
	wake := func() { postThreadQuitMessage(mainThreadID) }

	go func() {
		select {
		case <-readyCh:
		case <-stopCh:
		case <-time.After(trayReadyTimeout):
			readyTimedOut.Store(true)
			log.Printf("tray did not become ready within %s; waking event loop", trayReadyTimeout)
			wake()
		}
	}()

	onReady := func() {
		if icoBytes, icoErr := icon.GenerateICOFromSVG(logoSVGBytes); icoErr == nil {
			systray.SetIcon(icoBytes)
		} else {
			log.Printf("generate tray icon from svg: %v", icoErr)
		}
		systray.SetTitle("")
		systray.SetTooltip("MMonitor - Mihomo 流量监控")
		openItem := systray.AddMenuItem("打开监控面板", "在默认浏览器中打开 MMonitor")
		autoStartItem := systray.AddMenuItemCheckbox("开机自动启动", "登录 Windows 时自动启动 MMonitor", initialAutoStart)
		systray.AddSeparator()
		quitItem := systray.AddMenuItem("退出", "停止采集并将数据落盘后退出")
		menuCh <- trayMenu{
			openPageClicks:  openItem.ClickedCh,
			autoStartClicks: autoStartItem.ClickedCh,
			quitClicks:      quitItem.ClickedCh,
			setAutoStart: func(checked bool) {
				if checked {
					autoStartItem.Check()
				} else {
					autoStartItem.Uncheck()
				}
			},
		}
		close(readyCh)
	}
	onExit := func() {
		if err := app.shutdown(); err != nil {
			log.Printf("shutdown on tray exit: %v", err)
		}
	}

	runner.Run(onReady, onExit)
	close(stopCh)

	var runErr error
	select {
	case err := <-outcomeCh:
		runErr = err
	default:
	}
	if !quitRequested.Load() {
		if readyTimedOut.Load() {
			runErr = fmt.Errorf("tray initialization did not complete within %s", trayReadyTimeout)
		} else {
			runErr = errors.New("tray event loop returned without an explicit quit")
		}
		if err := app.shutdown(); err != nil {
			runErr = errors.Join(runErr, err)
		}
	}

	deleteRuntimeMetadataIfOwner(metaPath, os.Getpid(), sessionID)
	if err := logWriter.stickyError(); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("log writer: %w", err))
	}
	log.Printf("mmonitor exiting")
	_ = windowsLog.Close()
	return runErr
}

func reportPlatformFatal(err error) {
	log.Printf("fatal: %v", err)
	showErrorMessageBox(fmt.Sprintf("MMonitor 启动失败：\n\n%v\n\n详情请查看程序旁 data 目录下的 mmonitor.log。", err))
	if windowsLog != nil {
		_ = windowsLog.Close()
	}
}

func showErrorMessageBox(text string) {
	title, err := windows.UTF16PtrFromString("MMonitor")
	if err != nil {
		return
	}
	message, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, message, title, windows.MB_OK|windows.MB_ICONERROR)
}
