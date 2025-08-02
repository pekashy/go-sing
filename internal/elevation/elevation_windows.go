package elevation

import (
	"fmt"
	"go-sing/config"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	advapi32                = syscall.NewLazyDLL("advapi32.dll")
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	procGetTokenInformation = advapi32.NewProc("GetTokenInformation")
	procShellExecuteW       = shell32.NewProc("ShellExecuteW")
)

const (
	TOKEN_QUERY            = 0x0008
	TokenElevationType     = 18
	TokenElevationTypeFull = 2
	SW_HIDE                = 0
	SW_SHOW                = 5
)

func IsGoSingElevated() bool {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return false
	}

	var token syscall.Token
	err = syscall.OpenProcessToken(handle, TOKEN_QUERY, &token)
	if err != nil {
		return false
	}
	defer token.Close()

	var elevationType uint32
	var returnedLen uint32

	ret, _, _ := procGetTokenInformation.Call(
		uintptr(token),
		uintptr(TokenElevationType),
		uintptr(unsafe.Pointer(&elevationType)),
		uintptr(unsafe.Sizeof(elevationType)),
		uintptr(unsafe.Pointer(&returnedLen)),
	)

	return ret != 0 && elevationType == TokenElevationTypeFull
}

func RunElevated(program string, args string, workingDir string, showWindow bool) error {
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	var show int32 = SW_HIDE
	if showWindow {
		show = SW_SHOW
	}

	programPtr, err := syscall.UTF16PtrFromString(program)
	if err != nil {
		return fmt.Errorf("failed to convert program path: %w", err)
	}

	argsPtr, err := syscall.UTF16PtrFromString(args)
	if err != nil {
		return fmt.Errorf("failed to convert arguments: %w", err)
	}

	workingDirPtr, err := syscall.UTF16PtrFromString(workingDir)
	if err != nil {
		return fmt.Errorf("failed to convert working directory: %w", err)
	}

	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return fmt.Errorf("failed to convert verb: %w", err)
	}

	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(programPtr)),
		uintptr(unsafe.Pointer(argsPtr)),
		uintptr(unsafe.Pointer(workingDirPtr)),
		uintptr(show),
	)

	if ret <= 32 {

		switch ret {
		case 5:
			return fmt.Errorf("access denied - UAC prompt may have been cancelled")
		case 8:
			return fmt.Errorf("insufficient memory")
		case 31:
			return fmt.Errorf("no application associated with file")
		default:
			return fmt.Errorf("ShellExecute failed with code: %d", ret)
		}
	}

	return nil
}

func LaunchSingBoxElevated(appDir string) error {
	dataDir := filepath.Join(appDir, config.GoSingDataDir)
	singBoxPath := filepath.Join(dataDir, config.SingBoxExeName)
	configPath := filepath.Join(dataDir, config.SingBoxConfigFile)
	logsDir := filepath.Join(dataDir, config.SingBoxLogDir)
	logFilePath := filepath.Join(logsDir, config.SingBoxLogFile)

	if _, err := os.Stat(singBoxPath); os.IsNotExist(err) {
		return fmt.Errorf("sing-box.exe not found at: %s", singBoxPath)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config.json not found at: %s", configPath)
	}

	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	if err := os.WriteFile(logFilePath, []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}

	cmdArgs := fmt.Sprintf("/C \"\"%s\" run -c \"%s\" -D \"%s\" > \"%s\" 2>&1\"",
		singBoxPath, configPath, appDir, logFilePath)

	return RunElevated("cmd.exe", cmdArgs, appDir, false)
}

func KillSingBoxProcessElevated() error {

	args := fmt.Sprintf("/C taskkill /F /IM %s", config.SingBoxExeName)
	return RunElevated("cmd.exe", args, "", false)
}
