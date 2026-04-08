package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	notify_send_wrapper "github.com/Imnotndesh/notify-send-wrapper"
	"github.com/adrg/xdg"
	"github.com/fsnotify/fsnotify"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	ConfigFile  string
	EnvFile     string
	HyprFile    string
	HyprEnvFile string
	NoDaemon    bool
	NoExport    bool
	Verbose     bool
	Debug       bool
}

type TomlMap map[string]interface{}

type EnvVars []string
type HyprVars []string

var (
	config Config

	hydeConfigNotificationID = 19

	isInitialStartup = true

	noStartup bool // --no-startup flag
)

func main() {

	flag.StringVar(&config.ConfigFile, "input", filepath.Join(xdg.ConfigHome, "hyde", "config.toml"),
		"The input TOML file to parse. Default is $XDG_CONFIG_HOME/hyde/config.toml")
	flag.StringVar(&config.EnvFile, "env", filepath.Join(xdg.StateHome, "hyde", "config"),
		"The output environment file. Default is $XDG_STATE_HOME/hyde/config")
	flag.StringVar(&config.HyprFile, "hypr", filepath.Join(xdg.StateHome, "hyde", "hyprland.conf"),
		"The output Hyprland file. Default is $XDG_STATE_HOME/hyde/hyprland.conf")
	flag.StringVar(&config.HyprEnvFile, "hypr-env", filepath.Join(xdg.StateHome, "hyde", "hypr.conf"),
		"The output Hyprland env variables file. Default is $XDG_STATE_HOME/hyde/hypr.conf")
	flag.BoolVar(&config.NoDaemon, "no-daemon", false, "Run in one-off mode without watching for changes (daemon mode is default)")
	flag.BoolVar(&config.NoExport, "no-export", false, "Disable exporting the parsed data (export is default)")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&config.Debug, "debug", false, "Enable debug mode with detailed logging")
	flag.BoolVar(&noStartup, "no-startup", false, "Do not populate configs on startup, only parse and report errors")
	flag.Parse()

	if config.Verbose || config.Debug {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	} else {
		log.SetFlags(0)
	}
	log.SetPrefix("hyde-config: ")

	logInfo("Using config file: %s", config.ConfigFile)
	logInfo("Using env output file: %s", config.EnvFile)
	logInfo("Using hypr output file: %s", config.HyprFile)
	logInfo("Using hypr env output file: %s", config.HyprEnvFile)
	logInfo("Export mode: %v", !config.NoExport)
	logInfo("Daemon mode: %v", !config.NoDaemon)
	logInfo("Debug mode: %v", config.Debug)

	ensureDirExists(filepath.Dir(config.EnvFile))
	ensureDirExists(filepath.Dir(config.HyprFile))
	ensureDirExists(filepath.Dir(config.HyprEnvFile))

	if noStartup {
		logInfo("--no-startup: Only parsing config and reporting errors, not populating configs on startup")
		tomlContent, err := loadTomlFile(config.ConfigFile)
		if err != nil {
			logError("Startup parse error: %v", err)
			os.Exit(1)
		}
		if len(tomlContent) == 0 {
			logError("Startup TOML content is empty")
			os.Exit(1)
		}
		logInfo("Config parsed successfully on startup (no configs written)")
		// continue to daemon mode if enabled
	}

	if !config.NoDaemon {
		logInfo("Starting daemon mode, watching %s for changes", config.ConfigFile)
		watchFile(config.ConfigFile, config.EnvFile, config.HyprFile, config.HyprEnvFile, !config.NoExport)
	} else if !noStartup {
		parseConfigFiles(config.ConfigFile, config.EnvFile, config.HyprFile, config.HyprEnvFile, !config.NoExport)
		logInfo("Running in one-off mode (no watching for changes)")
	}
}

func ensureDirExists(dir string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}
}

func parseConfigFiles(tomlFile, envFile, hyprFile, hyprEnvFile string, exportMode bool) bool {

	fileInfo, err := os.Stat(tomlFile)
	if err != nil {
		logError("Failed to stat config file: %v", err)
		return false
	}

	if fileInfo.Size() == 0 {
		logError("Config file is empty, skipping parse")
		return false
	}

	tomlContent, err := loadTomlFile(tomlFile)
	if err != nil {

		return false
	}
	if len(tomlContent) == 0 {
		logError("TOML content is empty, skipping parse")
		return false
	}

	logDebug("TOML content loaded successfully, size of map: %d", len(tomlContent))

	var wg sync.WaitGroup
	wg.Add(3)

	var success1, success2, success3 bool

	go func() {
		defer wg.Done()
		success1 = parseTomlToEnvWithContent(tomlContent, envFile, exportMode)
	}()

	go func() {
		defer wg.Done()
		success2 = parseTomlToHyprWithContent(tomlContent, hyprFile)
	}()

	go func() {
		defer wg.Done()
		success3 = parseTomlToHyprEnvWithContent(tomlContent, hyprEnvFile)
	}()

	wg.Wait()

	success := success1 && success2 && success3

	if success && !config.NoDaemon && !isInitialStartup {
		sendSuccessNotification("Hyde Config", "Configuration reloaded successfully")
	}

	if isInitialStartup {
		isInitialStartup = false
	}

	return success
}

func logInfo(format string, v ...interface{}) {
	if config.Verbose || config.Debug {
		log.Printf(format, v...)
	}
}

func logDebug(format string, v ...interface{}) {
	if config.Debug {
		log.Printf("DEBUG: "+format, v...)
	}
}

func logError(format string, v ...interface{}) {
	errorMsg := fmt.Sprintf(format, v...)
	log.Printf("ERROR: " + errorMsg)

}

func sendSuccessNotification(title, message string) {

	if config.NoDaemon {
		return
	}

	notification := notify_send_wrapper.NewNotification().
		SetSummary(title).
		SetBody(message).
		SetUrgency(notify_send_wrapper.UrgencyNormal).
		SetIcon("dialog-information").
		SetAppName("hyde-config").
		SetCategory(notify_send_wrapper.CategorySystem).
		SetExpireTime(5000).
		SetReplaceID(hydeConfigNotificationID)

	go func() {
		err := notification.Send()
		if err != nil {
			logDebug("Failed to send notification: %v", err)
		}
	}()
}

func logTomlError(err error, filePath string) {
	errorMessage := err.Error()
	fileName := filepath.Base(filePath)

	var detailedMessage string
	if strings.Contains(errorMessage, "line") && strings.Contains(errorMessage, "column") {

		detailedMessage = fmt.Sprintf("TOML Syntax Error in %s:\n\n%s\n\nPlease check the file for syntax errors and try again.", fileName, errorMessage)
	} else if strings.Contains(errorMessage, "line") {

		detailedMessage = fmt.Sprintf("TOML Parse Error in %s:\n\n%s", fileName, errorMessage)
	} else {

		detailedMessage = fmt.Sprintf("TOML Error in %s:\n\n%s", fileName, errorMessage)
	}

	log.Printf("ERROR: %s", detailedMessage)

	if !config.NoDaemon {

		notificationMsg := detailedMessage
		if len(notificationMsg) > 250 {
			notificationMsg = detailedMessage[:250] + "..."
		}

		notification := notify_send_wrapper.NewNotification().
			SetSummary("Hyde Config: TOML Syntax Error").
			SetBody(notificationMsg).
			SetUrgency(notify_send_wrapper.UrgencyNormal).
			SetIcon("dialog-error").
			SetAppName("hyde-config").
			SetCategory(notify_send_wrapper.CategorySystem).
			SetExpireTime(20000).
			SetReplaceID(hydeConfigNotificationID).
			AddHint(notify_send_wrapper.HintResident, "true")

		go func() {
			err := notification.Send()
			if err != nil {
				logDebug("Failed to send TOML error notification: %v", err)
			}
		}()
	}
}

func loadTomlFile(tomlFile string) (TomlMap, error) {

	_, err := os.Stat(tomlFile)
	if err != nil {
		return nil, fmt.Errorf("failed to access TOML file: %w", err)
	}

	data, err := os.ReadFile(tomlFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read TOML file: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("TOML file is empty")
	}

	logDebug("Read %d bytes from TOML file", len(data))

	var tomlContent TomlMap
	if err := toml.Unmarshal(data, &tomlContent); err != nil {

		logTomlError(err, tomlFile)
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	return tomlContent, nil
}

func parseTomlToEnvWithContent(tomlContent TomlMap, envFile string, exportMode bool) bool {
	ignoredKeys := map[string]bool{
		"$schema":        true,
		"$SCHEMA":        true,
		"hyprland":       true,
		"hyprland-ipc":   true,
		"hyprland-env":   true,
		"hyprland-start": true,
	}

	if len(tomlContent) == 0 {
		logError("Cannot parse empty TOML content")
		return false
	}

	envVars := make(EnvVars, 0, len(tomlContent)*2)
	flattenDict(tomlContent, "", ignoredKeys, &envVars, exportMode)

	if len(envVars) == 0 {
		logError("No environment variables generated, skipping file write")
		return false
	}

	logDebug("Generated %d environment variable lines", len(envVars))

	if envFile != "" {

		// Use a unique temp file name to avoid race conditions
		tempFile := fmt.Sprintf("%s.tmp.%d.%d", envFile, os.Getpid(), time.Now().UnixNano())
		if err := writeLinesToFile(tempFile, envVars); err != nil {
			logError("Failed to write environment variables to temp file: %v", err)
			return false
		}

		if err := os.Rename(tempFile, envFile); err != nil {
			logError("Failed to replace environment file: %v", err)
			os.Remove(tempFile)
			return false
		}

		logInfo("Environment variables have been written to %s", envFile)
		return true
	} else {
		for _, line := range envVars {
			logInfo("%s", line)
		}
		return true
	}
}

func flattenDict(data TomlMap, parentKey string, ignoredKeys map[string]bool, result *EnvVars, exportMode bool) {
	for k, v := range data {

		if ignoredKeys[k] || (parentKey != "" && strings.HasPrefix(parentKey, "hyprland")) {
			logDebug("Skipping ignored key: %s", k)
			continue
		}

		if strings.HasPrefix(k, "$") {
			continue
		}

		var newKey string
		if parentKey != "" {
			newKey = fmt.Sprintf("%s_%s", parentKey, strings.ToUpper(k))
		} else {
			newKey = strings.ToUpper(k)
		}

		switch val := v.(type) {
		case map[string]interface{}:

			flattenDict(TomlMap(val), newKey, ignoredKeys, result, exportMode)
		case []interface{}:

			arrayItems := make([]string, 0, len(val))
			for _, item := range val {
				arrayItems = append(arrayItems, fmt.Sprintf("\"%v\"", item))
			}
			value := fmt.Sprintf("(%s)", strings.Join(arrayItems, " "))
			if exportMode {
				*result = append(*result, fmt.Sprintf("export %s=%s", newKey, value))
			} else {
				*result = append(*result, fmt.Sprintf("%s=%s", newKey, value))
			}
		case bool:

			value := strconv.FormatBool(val)
			if exportMode {
				*result = append(*result, fmt.Sprintf("export %s=%s", newKey, value))
			} else {
				*result = append(*result, fmt.Sprintf("%s=%s", newKey, value))
			}
		case int64, float64:

			if exportMode {
				*result = append(*result, fmt.Sprintf("export %s=%v", newKey, val))
			} else {
				*result = append(*result, fmt.Sprintf("%s=%v", newKey, val))
			}
		default:

			if exportMode {
				*result = append(*result, fmt.Sprintf("export %s=\"%v\"", newKey, val))
			} else {
				*result = append(*result, fmt.Sprintf("%s=\"%v\"", newKey, val))
			}
		}
	}
}

func parseTomlToHyprWithContent(tomlContent TomlMap, hyprFile string) bool {
	if len(tomlContent) == 0 {
		logError("Cannot parse empty TOML content for Hyprland")
		return false
	}

	hyprVars := make(HyprVars, 0, 32)
	flattenHyprDict(tomlContent, "", &hyprVars)

	if len(hyprVars) == 0 {
		logError("No Hyprland variables generated, skipping file write")
		return false
	}

	logDebug("Generated %d Hyprland variable lines", len(hyprVars))

	if hyprFile != "" {

		// Use a unique temp file name to avoid race conditions
		tempFile := fmt.Sprintf("%s.tmp.%d.%d", hyprFile, os.Getpid(), time.Now().UnixNano())
		if err := writeLinesToFile(tempFile, hyprVars); err != nil {
			logError("Failed to write Hyprland variables to temp file: %v", err)
			return false
		}

		if err := os.Rename(tempFile, hyprFile); err != nil {
			logError("Failed to replace Hyprland file: %v", err)
			os.Remove(tempFile)
			return false
		}

		logInfo("Hyprland variables have been written to %s", hyprFile)
		return true
	} else {
		logInfo("No hypr file specified.")
		for _, line := range hyprVars {
			logInfo("%s", line)
		}
		return true
	}
}

func flattenHyprDict(data TomlMap, parentKey string, result *HyprVars) {
	for k, v := range data {

		isHyprlandSection := strings.HasPrefix(k, "hyprland") || strings.HasPrefix(parentKey, "hyprland")

		if isHyprlandSection {
			logDebug("Found hyprland key: %s", k)

			newKey := k
			if strings.HasPrefix(newKey, "hyprland_") {
				newKey = strings.Replace(newKey, "hyprland_", "", 1)
			}

			if parentKey != "" && !strings.HasPrefix(parentKey, "hyprland") {
				newKey = fmt.Sprintf("%s_%s", parentKey, newKey)
			} else if strings.HasPrefix(parentKey, "hyprland") {
				if len(parentKey) > 9 {
					newKey = fmt.Sprintf("$%s.%s", parentKey[9:], strings.ToUpper(newKey))
				} else {
					newKey = fmt.Sprintf("$%s", strings.ToUpper(newKey))
				}
			}

			switch val := v.(type) {
			case map[string]interface{}:
				flattenHyprDict(TomlMap(val), newKey, result)
			case []interface{}:
				arrayItems := make([]string, 0, len(val))
				for _, item := range val {
					arrayItems = append(arrayItems, fmt.Sprintf("%v", item))
				}
				value := strings.Join(arrayItems, ", ")
				*result = append(*result, fmt.Sprintf("%s=%s", newKey, value))
			case bool:
				*result = append(*result, fmt.Sprintf("%s=%t", newKey, val))
			case int64, float64:
				*result = append(*result, fmt.Sprintf("%s=%v", newKey, val))
			default:
				*result = append(*result, fmt.Sprintf("%s=%v", newKey, val))
			}
		} else {
			logDebug("Skipping key: %s", k)
		}
	}
}

func parseTomlToHyprEnvWithContent(tomlContent TomlMap, hyprEnvFile string) bool {
	if len(tomlContent) == 0 {
		logError("Cannot parse empty TOML content for Hyprland env vars")
		return false
	}

	ignoredKeys := map[string]bool{
		"$schema":        true,
		"$SCHEMA":        true,
		"hyprland":       true,
		"hyprland-ipc":   true,
		"hyprland-env":   true,
		"hyprland-start": true,
	}

	hyprEnvVars := make(HyprVars, 0, len(tomlContent)*2+2)
	hyprEnvVars = append(hyprEnvVars, "# hyprlang noerror true")
	flattenToHyprVars(tomlContent, "", ignoredKeys, &hyprEnvVars)
	hyprEnvVars = append(hyprEnvVars, "# hyprlang noerror false")

	if len(hyprEnvVars) <= 2 {
		logError("No Hyprland env variables generated, skipping file write")
		return false
	}

	logDebug("Generated %d Hyprland env variable lines", len(hyprEnvVars))

	if hyprEnvFile != "" {
		tempFile := fmt.Sprintf("%s.tmp.%d.%d", hyprEnvFile, os.Getpid(), time.Now().UnixNano())
		if err := writeLinesToFile(tempFile, hyprEnvVars); err != nil {
			logError("Failed to write Hyprland env variables to temp file: %v", err)
			return false
		}

		if err := os.Rename(tempFile, hyprEnvFile); err != nil {
			logError("Failed to replace Hyprland env file: %v", err)
			os.Remove(tempFile)
			return false
		}

		logInfo("Hyprland env variables have been written to %s", hyprEnvFile)
		return true
	} else {
		logInfo("No hypr-env file specified.")
		for _, line := range hyprEnvVars {
			logInfo("%s", line)
		}
		return true
	}
}

func flattenToHyprVars(data TomlMap, parentKey string, ignoredKeys map[string]bool, result *HyprVars) {
	for k, v := range data {
		if ignoredKeys[k] || strings.HasPrefix(k, "hyprland") || strings.HasPrefix(parentKey, "hyprland") {
			logDebug("Skipping key for hypr-env: %s", k)
			continue
		}

		if strings.HasPrefix(k, "$") {
			continue
		}

		var newKey string
		if parentKey != "" {
			newKey = fmt.Sprintf("%s_%s", parentKey, strings.ToUpper(k))
		} else {
			newKey = strings.ToUpper(k)
		}

		switch val := v.(type) {
		case map[string]interface{}:
			flattenToHyprVars(TomlMap(val), newKey, ignoredKeys, result)
		case []interface{}:
			arrayItems := make([]string, 0, len(val))
			for _, item := range val {
				arrayItems = append(arrayItems, fmt.Sprintf("%v", item))
			}
			*result = append(*result, fmt.Sprintf("$%s = %s", newKey, strings.Join(arrayItems, ", ")))
		case bool:
			*result = append(*result, fmt.Sprintf("$%s = %t", newKey, val))
		case int64, float64:
			*result = append(*result, fmt.Sprintf("$%s = %v", newKey, val))
		default:
			*result = append(*result, fmt.Sprintf("$%s = %v", newKey, val))
		}
	}
}

func writeLinesToFile(filename string, lines []string) error {
	if len(lines) == 0 {
		return fmt.Errorf("no lines to write")
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filename, err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush file buffer: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}

	return nil
}

var (
	watcherMutex sync.Mutex
	lastMod      time.Time
)

func watchFile(tomlFile, envFile, hyprFile, hyprEnvFile string, exportMode bool) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logError("Failed to create file watcher: %v", err)
		return
	}
	defer watcher.Close()

	configDir := filepath.Dir(tomlFile)
	err = watcher.Add(configDir)
	if err != nil {
		logError("Failed to watch directory %s: %v", configDir, err)
		return
	}

	logInfo("Watching directory %s for changes to %s", configDir, filepath.Base(tomlFile))

	watcherMutex.Lock()
	lastMod = time.Now()
	watcherMutex.Unlock()

	debounceInterval := 300 * time.Millisecond
	configFileName := filepath.Base(tomlFile)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if filepath.Base(event.Name) != configFileName {
				continue
			}

			logDebug("Received event %s for file %s", event.Op, event.Name)

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {

				info, err := os.Stat(tomlFile)
				if err != nil {
					logError("Failed to stat file: %v", err)
					continue
				}

				watcherMutex.Lock()
				shouldProcess := time.Since(lastMod) > debounceInterval
				if shouldProcess {
					lastMod = info.ModTime()
				}
				watcherMutex.Unlock()

				if shouldProcess {
					logInfo("Config file changed (size: %d bytes), reprocessing", info.Size())

					time.Sleep(50 * time.Millisecond)

					parseConfigFiles(tomlFile, envFile, hyprFile, hyprEnvFile, exportMode)
				} else {
					logDebug("Skipping event, within debounce interval")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logError("Watcher error: %v", err)
		}
	}
}
